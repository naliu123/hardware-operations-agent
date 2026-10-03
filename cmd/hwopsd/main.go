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
	"hwops/internal/domain"
	"hwops/internal/observability"
	"hwops/internal/transport/httpapi"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	token := os.Getenv("HWOPS_API_TOKEN")
	if strings.TrimSpace(token) == "" {
		return errors.New("HWOPS_API_TOKEN is required")
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
	options := application.Options{}
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

	listener, err := net.Listen("tcp", env("HWOPS_LISTEN_ADDR", "127.0.0.1:8080"))
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: httpapi.New(app, token), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute,
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
