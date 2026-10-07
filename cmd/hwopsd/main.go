package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/model"

	"hwops/internal/adapters/chatmodel"
	"hwops/internal/adapters/elasticsearch"
	"hwops/internal/adapters/filestore"
	"hwops/internal/adapters/monitor"
	"hwops/internal/adapters/postgres"
	"hwops/internal/application"
	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/identity"
	"hwops/internal/observability"
	"hwops/internal/sandbox"
	"hwops/internal/transport/httpapi"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	authMode := env("HWOPS_AUTH_MODE", "local_token")
	if authMode != "local_token" && authMode != "users" {
		return errors.New("HWOPS_AUTH_MODE must be local_token or users")
	}
	token := os.Getenv("HWOPS_API_TOKEN")
	if authMode == "local_token" && strings.TrimSpace(token) == "" {
		return errors.New("HWOPS_API_TOKEN is required")
	}
	if authMode == "users" && (token != "" || os.Getenv("HWOPS_DATABASE_URL") == "") {
		return errors.New("users mode requires HWOPS_DATABASE_URL and rejects HWOPS_API_TOKEN")
	}
	mode := strings.ToUpper(env("HWOPS_MODEL_MODE", "LIVE"))
	var cm model.BaseChatModel
	switch mode {
	case "REPLAY":
		cm = &chatmodel.Replay{}
	case "LIVE":
		endpoint, name := os.Getenv("HWOPS_MODEL_ENDPOINT"), os.Getenv("HWOPS_MODEL")
		if endpoint == "" && name == "" {
			cm = &chatmodel.Unconfigured{}
			log.Print("model is unconfigured; questions requiring generation will fail with MODEL_UNAVAILABLE")
		} else {
			var err error
			cm, err = chatmodel.NewOpenAI(endpoint, name, os.Getenv("HWOPS_MODEL_API_KEY"))
			if err != nil {
				return errors.New("set both HWOPS_MODEL_ENDPOINT (full chat/completions URL) and HWOPS_MODEL")
			}
		}
	default:
		return errors.New("HWOPS_MODEL_MODE must be LIVE or REPLAY")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	flush, err := observability.Configure(ctx, os.Getenv("HWOPS_PHOENIX_ENDPOINT"), env("HWOPS_PHOENIX_PROJECT", "hwops"))
	if err != nil {
		return err
	}
	defer flush()
	var store interface {
		domain.Repository
		Close() error
	}
	backend := "file"
	if dsn := os.Getenv("HWOPS_DATABASE_URL"); dsn != "" {
		backend = "postgres"
		connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		store, err = postgres.Open(connectCtx, dsn)
		cancel()
	} else {
		store, err = filestore.Open(env("HWOPS_STATE_PATH", ".local/state.json"))
	}
	if err != nil {
		return fmt.Errorf("open %s repository: %w", backend, err)
	}
	defer store.Close()
	options := application.Options{UsersMode: authMode == "users"}
	if options.UsersMode {
		options.ModelContextTokens, err = strconv.Atoi(env("HWOPS_MODEL_CONTEXT_TOKENS", "131072"))
		if err != nil {
			return errors.New("HWOPS_MODEL_CONTEXT_TOKENS must be an integer")
		}
		options.InputTokenBudget, err = strconv.Atoi(env("HWOPS_INPUT_TOKEN_BUDGET", "98304"))
		if err != nil {
			return errors.New("HWOPS_INPUT_TOKEN_BUDGET must be an integer")
		}
		quota, parseErr := strconv.ParseInt(env("HWOPS_FILE_QUOTA_BYTES", "10000000000"), 10, 64)
		if parseErr != nil {
			return errors.New("invalid private file quota")
		}
		reserve, parseErr := strconv.ParseInt(env("HWOPS_FILE_RESERVE_BYTES", "1000000000"), 10, 64)
		if parseErr != nil {
			return errors.New("invalid private file reserve")
		}
		options.Files, err = blobstore.Open(env("HWOPS_FILES_DIR", ".local/workbench-files"), quota, reserve)
		if err != nil {
			return err
		}
		if endpoint := os.Getenv("HWOPS_RUNNER_URL"); endpoint != "" {
			options.Runner, err = runnerClient(endpoint, os.Getenv("HWOPS_RUNNER_TOKEN_FILE"), "Python")
			if err != nil {
				return err
			}
		}
		if endpoint := os.Getenv("HWOPS_PARSER_RUNNER_URL"); endpoint != "" {
			options.AttachmentRunner, err = runnerClient(endpoint, os.Getenv("HWOPS_PARSER_RUNNER_TOKEN_FILE"), "attachment parser")
			if err != nil {
				return err
			}
		}
	}
	options.TracePrivateContent, err = envBool("HWOPS_TRACE_PRIVATE_CONTENT", false)
	if err != nil {
		return errors.New("HWOPS_TRACE_PRIVATE_CONTENT must be true or false")
	}
	if endpoint := os.Getenv("HWOPS_MONITOR_ENDPOINT"); endpoint != "" {
		options.Observer, err = monitor.New(endpoint, os.Getenv("HWOPS_MONITOR_API_KEY"), mode, 10*time.Second, 4)
		if err != nil {
			return err
		}
	}
	options.EvidenceSelection, err = envBool("HWOPS_EVIDENCE_SELECTION", false)
	if err != nil {
		return errors.New("HWOPS_EVIDENCE_SELECTION must be true or false")
	}
	options.QueryRewrite, err = envBool("HWOPS_QUERY_REWRITE", false)
	if err != nil {
		return errors.New("HWOPS_QUERY_REWRITE must be true or false")
	}
	if endpoint := os.Getenv("HWOPS_ES_URL"); endpoint != "" {
		rrf, err := strconv.Atoi(env("HWOPS_RRF_CONSTANT", "60"))
		if err != nil {
			return errors.New("HWOPS_RRF_CONSTANT must be an integer in 1..1000")
		}
		index, err := elasticsearch.New(endpoint, os.Getenv("HWOPS_EMBED_URL"),
			env("HWOPS_ES_INDEX", "hwops-fragments-v1"), env("HWOPS_RETRIEVAL_STRATEGY", "hybrid"), rrf)
		if err != nil {
			return err
		}
		multiVector, err := envBool("HWOPS_MULTI_VECTOR", false)
		if err != nil {
			return errors.New("HWOPS_MULTI_VECTOR must be true or false")
		}
		if multiVector {
			index.EnableMultiVector()
		}
		if err := index.Ensure(ctx); err != nil {
			return err
		}
		options.Retriever, options.ContextLimit = index, 5
	}
	app, err := application.New(store, cm, mode, options)
	if err != nil {
		return err
	}
	defer app.Close()

	var handler http.Handler
	if authMode == "users" {
		accounts, err := identity.New(store.(*postgres.Store).Pool())
		if err != nil {
			return err
		}
		handler, err = httpapi.NewUsers(app, accounts, httpapi.UsersConfig{
			Origin: os.Getenv("HWOPS_PUBLIC_ORIGIN"), StaticDir: env("HWOPS_WEB_DIR", "web/dist"),
			TracePrivateContent: options.TracePrivateContent,
		})
		if err != nil {
			return err
		}
	} else {
		handler = httpapi.New(app, token)
	}
	listener, err := net.Listen("tcp", env("HWOPS_LISTEN_ADDR", "127.0.0.1:8080"))
	if err != nil {
		return err
	}
	readTimeout, writeTimeout := 15*time.Second, 15*time.Second
	if authMode == "users" {
		readTimeout, writeTimeout = 2*time.Minute, 2*time.Minute
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: time.Minute,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	log.Printf("hwopsd listening on %s; data_mode=%s; repository=%s", listener.Addr(), mode, backend)
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}

func runnerClient(endpoint, tokenFile, label string) (sandbox.Runner, error) {
	info, err := os.Stat(tokenFile)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("%s token file must be private", label)
	}
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("%s token unavailable", label)
	}
	runner, err := sandbox.NewClient(endpoint, strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s runner: %w", label, err)
	}
	return runner, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseBool(value)
}
