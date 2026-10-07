//go:build linux

package sandbox

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"hwops/internal/domain"
)

type daemon struct {
	cfg    Config
	mu     sync.Mutex
	cap    Capability
	binary string
	docker string
}

func Serve(ctx context.Context, cfg Config) error {
	if os.Geteuid() != 0 || !digest.MatchString(cfg.Image) || len(cfg.Token) < 32 ||
		!filepath.IsAbs(cfg.Root) || strings.ContainsAny(cfg.Root, " \t\n:%") {
		return errors.New("runner requires root, absolute private root, pinned image and token")
	}
	if err := os.MkdirAll(cfg.Root, 0700); err != nil {
		return err
	}
	rootInfo, err := os.Lstat(cfg.Root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm()&0077 != 0 {
		return errors.New("runner root must be a real private directory")
	}
	lock, err := os.OpenFile(filepath.Join(cfg.Root, "runner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("runner already active for this root")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	docker, err := exec.LookPath("docker")
	if err != nil || !filepath.IsAbs(docker) {
		return errors.New("docker executable unavailable")
	}
	d := &daemon{cfg: cfg, binary: binary, docker: docker}
	if err = os.MkdirAll(filepath.Join(cfg.Root, "jobs"), 0700); err != nil {
		return err
	}
	// Previous jobs are reconciled before readiness. No request is replayed.
	entries, err := os.ReadDir(filepath.Join(cfg.Root, "jobs"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !safeID.MatchString(entry.Name()) {
			return errors.New("unexpected runner journal entry")
		}
		state, err := d.read(entry.Name())
		if err != nil {
			return err
		}
		if !domain.PythonTerminal(state.State) || !state.Result.Cleaned {
			if _, err = d.stop(entry.Name(), "INTERRUPTED"); err != nil {
				return fmt.Errorf("cannot reconcile execution %s", entry.Name())
			}
		}
	}
	d.cap, err = Probe(ctx, cfg)
	if err != nil {
		return fmt.Errorf("isolation probe failed: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capability", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, d.cap) })
	mux.HandleFunc("PUT /v1/executions/{id}", d.submit)
	mux.HandleFunc("GET /v1/executions/{id}", d.get)
	mux.HandleFunc("POST /v1/executions/{id}/cancel", d.cancel)
	mux.HandleFunc("DELETE /v1/executions/{id}", d.forget)
	mux.HandleFunc("GET /v1/executions/{id}/artifacts/{artifact}", d.artifact)
	server := &http.Server{Addr: cfg.Listen, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute,
		WriteTimeout: 2 * time.Minute, IdleTimeout: time.Minute, MaxHeaderBytes: 8 << 10,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+cfg.Token)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
			mux.ServeHTTP(w, r)
		})}
	if cfg.TLSCert == "" || cfg.TLSKey == "" {
		host, _, err := net.SplitHostPort(cfg.Listen)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("remote runner control requires TLS")
		}
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if cfg.TLSCert != "" {
		err = server.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		err = server.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (d *daemon) dir(id string) string { return filepath.Join(d.cfg.Root, "jobs", id) }

func (d *daemon) read(id string) (Status, error) {
	var out Status
	if !safeID.MatchString(id) {
		return out, ErrMissing
	}
	err := readJSON(filepath.Join(d.dir(id), "status.json"), &out)
	if errors.Is(err, os.ErrNotExist) {
		err = ErrMissing
	}
	return out, err
}

func (d *daemon) submit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 140_000_000)
	defer r.Body.Close()
	var request Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF ||
		request.Validate() != nil || request.ID != r.PathValue("id") || request.Image != d.cfg.Image {
		http.Error(w, "invalid request, input digest or execution policy", 400)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	existing, err := d.read(request.ID)
	if err == nil {
		if existing.RequestSHA256 != request.Hash() {
			http.Error(w, "identity already reserved (or canceled)", 409)
		} else {
			writeJSON(w, 200, existing)
		}
		return
	}
	if !errors.Is(err, ErrMissing) || !time.Now().Before(request.Deadline) {
		http.Error(w, "execution unavailable or expired", 503)
		return
	}
	entries, err := os.ReadDir(filepath.Join(d.cfg.Root, "jobs"))
	if err != nil {
		http.Error(w, "journal unavailable", 503)
		return
	}
	active := 0
	for _, entry := range entries {
		state, err := d.read(entry.Name())
		if err != nil {
			http.Error(w, "journal inconsistent", 503)
			return
		}
		if !domain.PythonTerminal(state.State) || !state.Result.Cleaned {
			active++
		}
	}
	// Defense in depth: the application queue cannot bypass runner capacity.
	if active >= 2 {
		http.Error(w, "runner capacity reached", 503)
		return
	}
	dir := d.dir(request.ID)
	if err = os.Mkdir(dir, 0700); err != nil {
		http.Error(w, "journal unavailable", 503)
		return
	}
	state := Status{ID: request.ID, RequestSHA256: request.Hash(), State: "STARTING"}
	// First write the identity. Even if input staging fails this ID will never
	// start a different execution, or be retried by blindly starting a job.
	if err = atomicJSON(filepath.Join(dir, "status.json"), state); err == nil {
		err = stageRequest(dir, request)
	}
	if err == nil {
		// RuntimeMaxSec and ExecStopPost survive the controller's termination.
		// No credentials or application environment enter this service.
		_, err = command(r.Context(), 10*time.Second, "systemd-run", "--quiet", "--collect",
			"--unit="+unit(request.ID), "--property=Type=exec", "--property=RuntimeMaxSec=75s",
			"--property=TimeoutStopSec=10s", "--property=KillMode=control-group",
			"--property=ExecStopPost="+d.docker+" rm --force "+containerName(request.ID),
			"--property=UMask=0077", d.binary, "-root", d.cfg.Root, "-job", request.ID)
	}
	if err != nil {
		// An ambiguous systemd response is not proof the process never started.
		// stop() verifies cleanup before making the failure terminal.
		state, err = d.stop(request.ID, "INTERRUPTED")
		if err != nil {
			http.Error(w, "dispatch unknown; reconciliation required", 503)
			return
		}
	}
	writeJSON(w, 202, state)
}

func stageRequest(dir string, request Request) error {
	inputs := filepath.Join(dir, "inputs")
	if err := os.Mkdir(inputs, 0755); err != nil {
		return err
	}
	// The service deliberately runs with UMask=0077. Explicit chmod restores
	// only the bind-mounted read policy needed by the non-root container user;
	// the runner root and job journal remain private.
	if err := os.Chmod(inputs, 0555); err != nil {
		return err
	}
	code := filepath.Join(dir, "code.py")
	if err := os.WriteFile(code, []byte(request.Code), 0600); err != nil {
		return err
	}
	if err := os.Chmod(code, 0444); err != nil {
		return err
	}
	for i, input := range request.Inputs {
		name := filepath.Join(inputs, input.ID)
		if err := os.WriteFile(name, input.Data, 0600); err != nil {
			return err
		}
		if err := os.Chmod(name, 0444); err != nil {
			return err
		}
		request.Inputs[i].Data = nil
	}
	return atomicJSON(filepath.Join(dir, "request.json"), request)
}

func (d *daemon) get(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, err := d.read(r.PathValue("id"))
	if err != nil {
		http.Error(w, "execution unavailable", 404)
		return
	}
	if !domain.PythonTerminal(state.State) {
		// A killed job worker may not have written its final record. Reconcile
		// against systemd, then verify Docker removal before reporting terminal.
		raw, err := command(r.Context(), 3*time.Second, "systemctl", "show", unit(state.ID), "--property=ActiveState", "--value")
		if err == nil && stringTrim(raw) != "active" && stringTrim(raw) != "activating" && stringTrim(raw) != "deactivating" {
			state, err = d.stop(state.ID, "INTERRUPTED")
			if err != nil {
				http.Error(w, "execution cleanup unconfirmed", 503)
				return
			}
		}
	}
	writeJSON(w, 200, state)
}

func (d *daemon) cancel(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, err := d.stop(r.PathValue("id"), "CANCELED")
	if err != nil {
		http.Error(w, "execution cleanup unconfirmed", 503)
		return
	}
	writeJSON(w, 200, state)
}

func (d *daemon) stop(id, terminal string) (Status, error) {
	state, err := d.read(id)
	if !safeID.MatchString(id) || (err != nil && !errors.Is(err, ErrMissing)) {
		return state, ErrMissing
	}
	if err == nil && domain.PythonTerminal(state.State) && state.Result.Cleaned {
		return state, nil
	}
	if errors.Is(err, ErrMissing) {
		// A cancel arriving before a delayed submit leaves a durable tombstone.
		if err = os.MkdirAll(d.dir(id), 0700); err != nil {
			return state, err
		}
		state.ID = id
	}
	if err = os.WriteFile(filepath.Join(d.dir(id), "cancel"), []byte(terminal), 0600); err != nil {
		return state, err
	}
	state.State = "CANCELING"
	if err = atomicJSON(filepath.Join(d.dir(id), "status.json"), state); err != nil {
		return state, err
	}
	ctx := context.Background()
	_, _ = command(ctx, 15*time.Second, "systemctl", "stop", unit(id))
	if err = removeContainer(ctx, id); err != nil {
		return state, err
	}
	if err = cleanupWork(d.dir(id)); err != nil {
		return state, err
	}
	now := time.Now().UTC()
	state.State, state.Result.Cleaned, state.Result.Complete = terminal, true, false
	state.Result.Error, state.Result.FinishedAt = terminal, &now
	state.Result.Artifacts = nil
	err = atomicJSON(filepath.Join(d.dir(id), "status.json"), state)
	return state, err
}

func (d *daemon) artifact(w http.ResponseWriter, r *http.Request) {
	state, err := d.read(r.PathValue("id"))
	if err != nil || state.State != "SUCCEEDED" || !state.Result.Complete || !state.Result.Cleaned {
		http.Error(w, "artifact unavailable", 404)
		return
	}
	for _, a := range state.Result.Artifacts {
		if a.ID != r.PathValue("artifact") {
			continue
		}
		file, err := os.Open(filepath.Join(d.dir(state.ID), "artifacts", a.ID))
		if err != nil {
			http.Error(w, "artifact unavailable", 404)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, a.Name, time.Time{}, file)
		return
	}
	http.Error(w, "artifact unavailable", 404)
}

func (d *daemon) forget(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, err := d.stop(r.PathValue("id"), "CANCELED")
	if err == nil {
		for _, name := range []string{"artifacts", "inputs", "code.py", "request.json"} {
			if err = os.RemoveAll(filepath.Join(d.dir(state.ID), name)); err != nil {
				break
			}
		}
	}
	if err != nil {
		http.Error(w, "cleanup unconfirmed", 503)
		return
	}
	// Retain only identity/hash tombstone, preventing a delayed retry from
	// running code after the application deleted its execution.
	state.Result = domain.PythonResult{Cleaned: true}
	state.State = "CANCELED"
	if atomicJSON(filepath.Join(d.dir(state.ID), "status.json"), state) != nil {
		http.Error(w, "cleanup journal unavailable", 503)
		return
	}
	writeJSON(w, 200, state)
}

func unit(id string) string          { return "hwops-exec-" + id + ".service" }
func containerName(id string) string { return "hwops-" + id }
func stringTrim(raw []byte) string   { return strings.TrimSpace(string(raw)) }

func command(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// Controller/job services receive no inherited credentials.
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s failed", name)
	}
	return out, nil
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func atomicJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".journal-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err == nil {
		dir, openErr := os.Open(filepath.Dir(path))
		if openErr != nil {
			return openErr
		}
		defer dir.Close()
		err = dir.Sync()
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
