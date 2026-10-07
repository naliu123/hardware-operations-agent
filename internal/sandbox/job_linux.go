//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"hwops/internal/domain"
)

func verifyEnvironment(ctx context.Context, image string) error {
	var controllers []byte
	controllers, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if err != nil || !strings.Contains(string(controllers), "cpu") || !strings.Contains(string(controllers), "memory") ||
		!strings.Contains(string(controllers), "pids") {
		return errors.New("cgroup v2 cpu/memory/pids controllers required")
	}
	raw, err := command(ctx, 5*time.Second, "docker", "info", "--format", "{{json .Runtimes}}")
	if err != nil {
		return err
	}
	var runtimes map[string]struct {
		Path string   `json:"path"`
		Args []string `json:"runtimeArgs"`
	}
	if json.Unmarshal(raw, &runtimes) != nil || runtimes["hwops-runsc"].Path != "/usr/local/bin/runsc" {
		return errors.New("dedicated gVisor runtime not installed")
	}
	flags := strings.Join(runtimes["hwops-runsc"].Args, " ")
	if !strings.Contains(flags, "--network=none") || !strings.Contains(flags, "--platform=systrap") {
		return errors.New("gVisor network/platform configuration differs")
	}
	version, err := command(ctx, 5*time.Second, "/usr/local/bin/runsc", "--version")
	if err != nil || !strings.Contains(string(version), "20260928.0") {
		return errors.New("gVisor release differs from verified version")
	}
	actual, err := command(ctx, 5*time.Second, "docker", "image", "inspect", image, "--format", "{{.Id}}")
	if err != nil || stringTrim(actual) != image || !digest.MatchString(image) {
		return errors.New("pinned execution image unavailable")
	}
	return nil
}

// RunJob runs in a separate systemd service. The controller does not own its
// lifetime; systemd enforces RuntimeMaxSec and always removes the container.
func RunJob(root, id string) error {
	if os.Geteuid() != 0 || !safeID.MatchString(id) {
		return ErrUnavailable
	}
	dir := filepath.Join(root, "jobs", id)
	lock, err := os.OpenFile(filepath.Join(dir, "job.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("execution already running")
	}
	var request Request
	var state Status
	if err = readJSON(filepath.Join(dir, "request.json"), &request); err != nil {
		return err
	}
	if err = readJSON(filepath.Join(dir, "status.json"), &state); err != nil {
		return err
	}
	if state.State != "STARTING" || state.RequestSHA256 != request.Hash() || state.ID != request.ID {
		return errors.New("execution journal inconsistent or already consumed")
	}
	state = execute(dir, request)
	return atomicJSON(filepath.Join(dir, "status.json"), state)
}

type logCapture struct {
	mu             sync.Mutex
	stdout, stderr bytes.Buffer
	size           int64
	exceeded       bool
	cancel         context.CancelFunc
}

type logWriter struct {
	capture *logCapture
	stderr  bool
}

func (w logWriter) Write(p []byte) (int, error) {
	c := w.capture
	c.mu.Lock()
	defer c.mu.Unlock()
	remaining := domain.PythonLimits().LogBytes - c.size
	keep := int64(len(p))
	if keep > remaining {
		keep, c.exceeded = remaining, true
		c.cancel()
	}
	if w.stderr {
		c.stderr.Write(p[:keep])
	} else {
		c.stdout.Write(p[:keep])
	}
	c.size += keep
	return len(p), nil
}

func execute(dir string, request Request) (state Status) {
	state = Status{ID: request.ID, RequestSHA256: request.Hash(), State: "STARTING"}
	ctx := context.Background()
	now := time.Now().UTC()
	state.Result.StartedAt = &now
	finish := func(code string) Status {
		state.State = "FAILED"
		state.Result.Error = code
		return state
	}
	defer func() {
		// A terminal state is not written until all execution processes have
		// exited and temporary files have been released.
		if err := removeContainer(ctx, request.ID); err != nil {
			state.State, state.Result.Cleaned = "CANCELING", false
			state.Result.Error = "CLEANUP_UNCONFIRMED"
		} else if err = cleanupWork(dir); err != nil {
			state.State, state.Result.Cleaned = "CANCELING", false
			state.Result.Error = "CLEANUP_UNCONFIRMED"
		} else {
			state.Result.Cleaned = true
		}
		end := time.Now().UTC()
		state.Result.FinishedAt, state.Result.DurationMS = &end, end.Sub(now).Milliseconds()
		if reason, err := os.ReadFile(filepath.Join(dir, "cancel")); err == nil {
			state.State, state.Result.Error = string(reason), string(reason)
			state.Result.Complete, state.Result.Artifacts = false, nil
		}
	}()
	if err := verifyEnvironment(ctx, request.Image); err != nil {
		return finish("ISOLATION_UNAVAILABLE")
	}
	if _, err := os.Stat(filepath.Join(dir, "cancel")); err == nil || !time.Now().Before(request.Deadline) {
		return finish("EXECUTION_EXPIRED_OR_CANCELED")
	}
	work := filepath.Join(dir, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		return finish("WORKSPACE_UNAVAILABLE")
	}
	if err := unix.Mount("tmpfs", work, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC,
		"size=268435456,mode=0700,uid=65532,gid=65532"); err != nil {
		return finish("WORKSPACE_QUOTA_UNAVAILABLE")
	}
	for _, name := range []string{"tmp", "output"} {
		if err := os.Mkdir(filepath.Join(work, name), 0700); err != nil {
			return finish("WORKSPACE_UNAVAILABLE")
		}
		if err := os.Chown(filepath.Join(work, name), 65532, 65532); err != nil {
			return finish("WORKSPACE_UNAVAILABLE")
		}
	}
	args := []string{"create", "--name", containerName(request.ID), "--runtime", "hwops-runsc",
		"--network", "none", "--read-only", "--user", "65532:65532", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--cpus", "1", "--memory", "1073741824",
		"--memory-swap", "1073741824", "--pids-limit", "64", "--ulimit", "nproc=64:64",
		"--ulimit", "nofile=128:128", "--ulimit", "core=0:0", "--ipc", "none",
		"--log-driver", "none", "--stop-timeout", "0", "--workdir", "/work",
		"--mount", "type=bind,src=" + work + ",dst=/work",
		"--mount", "type=bind,src=" + filepath.Join(dir, "inputs") + ",dst=/inputs,readonly",
		"--mount", "type=bind,src=" + filepath.Join(dir, "code.py") + ",dst=/code.py,readonly",
		"--env", "HOME=/work", "--env", "TMPDIR=/work/tmp", "--env", "MPLCONFIGDIR=/work/tmp",
		"--env", "MPLBACKEND=Agg", "--env", "OPENBLAS_NUM_THREADS=1", "--env", "OMP_NUM_THREADS=1",
		"--entrypoint", "/usr/local/bin/python", request.Image, "-I", "-B", "/code.py"}
	if _, err := command(ctx, 5*time.Second, "docker", args...); err != nil {
		return finish("CONTAINER_CREATE_FAILED")
	}
	// Verify the effective Docker configuration before any Python starts.
	raw, err := command(ctx, 3*time.Second, "docker", "inspect", containerName(request.ID))
	containerID, policyErr := verifyContainer(raw)
	if err != nil || policyErr != nil {
		return finish("CONTAINER_POLICY_MISMATCH")
	}
	deadline := time.Now().Add(time.Duration(request.Budget.Seconds) * time.Second)
	if request.Deadline.Before(deadline) {
		deadline = request.Deadline
	}
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	capture := &logCapture{cancel: cancel}
	usageStop, usageDone := make(chan struct{}), make(chan usageObservation, 1)
	go monitorCgroup(containerID, usageStop, usageDone)
	cmd := exec.CommandContext(runCtx, "docker", "start", "--attach", containerName(request.ID))
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	cmd.Stdout, cmd.Stderr = logWriter{capture, false}, logWriter{capture, true}
	state.State = "RUNNING"
	if atomicJSON(filepath.Join(dir, "status.json"), state) != nil {
		return finish("JOURNAL_UNAVAILABLE")
	}
	runErr := cmd.Run()
	close(usageStop)
	observed := <-usageDone
	state.Result.Stdout, state.Result.Stderr = capture.stdout.String(), capture.stderr.String()
	inspect, inspectErr := command(ctx, 3*time.Second, "docker", "inspect", containerName(request.ID),
		"--format", "{{json .State}}")
	var exit struct {
		Running   bool
		ExitCode  int
		OOMKilled bool
		Error     string
	}
	if inspectErr == nil {
		inspectErr = json.Unmarshal(inspect, &exit)
		if inspectErr == nil && !exit.Running {
			state.Result.ExitCode = &exit.ExitCode
		}
	}
	state.Result.ResourceUsage = observed.usage
	if !observed.ok {
		return finish("RESOURCE_ACCOUNTING_UNAVAILABLE")
	}
	// Remove all descendants before looking at user-controlled output paths.
	if err := removeContainer(ctx, request.ID); err != nil {
		return finish("CLEANUP_UNCONFIRMED")
	}
	switch {
	case capture.exceeded:
		return finish("OUTPUT_LIMIT_EXCEEDED")
	case runCtx.Err() != nil:
		return finish("TIME_LIMIT_EXCEEDED")
	case inspectErr != nil || exit.Running:
		return finish("EXIT_UNCONFIRMED")
	case exit.OOMKilled:
		return finish("MEMORY_LIMIT_EXCEEDED")
	case state.Result.ResourceUsage.ProcessLimitEvents > 0:
		return finish("PROCESS_LIMIT_EXCEEDED")
	case exit.ExitCode != 0 || runErr != nil || exit.Error != "":
		return finish("PYTHON_FAILED")
	}
	state.Result.Artifacts, err = collectArtifacts(dir, request.ID)
	if err != nil {
		return finish("ARTIFACT_VALIDATION_FAILED")
	}
	state.State, state.Result.Complete = "SUCCEEDED", true
	return state
}

func verifyContainer(raw []byte) (string, error) {
	var containers []struct {
		ID     string
		Config struct {
			User string
			Env  []string
		}
		HostConfig struct {
			Runtime, NetworkMode, IpcMode           string
			ReadonlyRootfs, Privileged              bool
			NanoCpus, Memory, MemorySwap, PidsLimit int64
			CapDrop, CapAdd, SecurityOpt            []string
		}
	}
	if json.Unmarshal(raw, &containers) != nil || len(containers) != 1 {
		return "", ErrUnavailable
	}
	c, h := containers[0].Config, containers[0].HostConfig
	if !containerIDPattern.MatchString(containers[0].ID) ||
		c.User != "65532:65532" || h.Runtime != "hwops-runsc" || h.NetworkMode != "none" ||
		!h.ReadonlyRootfs || h.Privileged || h.NanoCpus != 1e9 || h.Memory != 1<<30 ||
		h.MemorySwap != 1<<30 || h.PidsLimit != 64 || h.IpcMode != "none" ||
		len(h.CapAdd) != 0 || strings.Join(h.CapDrop, ",") != "ALL" ||
		!strings.Contains(strings.Join(h.SecurityOpt, ","), "no-new-privileges") {
		return "", ErrUnavailable
	}
	return containers[0].ID, nil
}

var containerIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func cgroupUsage(id string) (domain.ResourceUsage, error) {
	if !containerIDPattern.MatchString(id) {
		return domain.ResourceUsage{}, ErrUnavailable
	}
	dir := filepath.Join("/sys/fs/cgroup/docker", id)
	readInt := func(name string) (int64, error) {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return 0, err
		}
		value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil || value < 0 {
			return 0, ErrUnavailable
		}
		return value, nil
	}
	usage := domain.ResourceUsage{}
	var err error
	if usage.MemoryPeakBytes, err = readInt("memory.peak"); err != nil {
		return usage, err
	}
	if usage.ProcessesPeak, err = readInt("pids.peak"); err != nil {
		return usage, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "pids.events"))
	if err != nil {
		return usage, err
	}
	if _, err = fmt.Sscanf(string(raw), "max %d", &usage.ProcessLimitEvents); err != nil {
		return usage, err
	}
	raw, err = os.ReadFile(filepath.Join(dir, "cpu.stat"))
	if err != nil {
		return usage, err
	}
	if _, err = fmt.Sscanf(string(raw), "usage_usec %d", &usage.CPUUsec); err != nil {
		return usage, err
	}
	return usage, nil
}

type usageObservation struct {
	usage domain.ResourceUsage
	ok    bool
}

func monitorCgroup(id string, stop <-chan struct{}, done chan<- usageObservation) {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	last := usageObservation{}
	for {
		if usage, err := cgroupUsage(id); err == nil {
			last.usage = usage
			last.ok = true
		}
		select {
		case <-stop:
			done <- last
			return
		case <-ticker.C:
		}
	}
}

func removeContainer(ctx context.Context, id string) error {
	// Always query the daemon after removal. A CLI timeout or daemon outage is
	// not evidence of cleanup, including when rm reports "not found".
	_, _ = command(ctx, 5*time.Second, "docker", "rm", "--force", containerName(id))
	raw, err := command(ctx, 3*time.Second, "docker", "ps", "-aq", "--filter", "name=^/"+containerName(id)+"$")
	if err != nil || strings.TrimSpace(string(raw)) != "" {
		return ErrUnavailable
	}
	return nil
}

func cleanupWork(dir string) error {
	work := filepath.Join(dir, "work")
	if err := unix.Unmount(work, 0); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return os.RemoveAll(work)
}

func collectArtifacts(dir, id string) ([]domain.Artifact, error) {
	output := filepath.Join(dir, "work", "output")
	info, err := os.Lstat(output)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("output must be a real directory")
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) > domain.PythonLimits().ArtifactCount {
		return nil, errors.New("too many artifacts")
	}
	if err = os.MkdirAll(filepath.Join(dir, "artifacts"), 0700); err != nil {
		return nil, err
	}
	artifacts, total := []domain.Artifact{}, int64(0)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Nlink != 1 ||
			len(entry.Name()) > 180 || strings.ContainsAny(entry.Name(), "\x00\r\n") {
			return nil, errors.New("artifact is not a single regular file")
		}
		total += info.Size()
		if total > domain.PythonLimits().ArtifactBytes {
			return nil, errors.New("artifact byte limit exceeded")
		}
		data, err := os.ReadFile(filepath.Join(output, entry.Name()))
		if err != nil || int64(len(data)) != info.Size() {
			return nil, errors.New("incomplete artifact")
		}
		a := domain.Artifact{ID: rand.Text(), ExecutionID: id, Name: entry.Name(), Bytes: info.Size(),
			SHA256: Hash(data), MediaType: http.DetectContentType(data)}
		// HTML/SVG and other active formats are always downloaded as bytes.
		if strings.Contains(a.MediaType, "html") || strings.Contains(a.MediaType, "svg") {
			a.MediaType = "application/octet-stream"
		}
		if err = os.WriteFile(filepath.Join(dir, "artifacts", a.ID), data, 0600); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, nil
}

const probeCode = `import os, socket, resource, json
import numpy, pandas, matplotlib, PIL
try:
    import pymupdf
    pymupdf_version = pymupdf.__version__
except ImportError:
    pymupdf_version = ""
assert os.getuid() == 65532
assert resource.getrlimit(resource.RLIMIT_NPROC) == (64,64)
assert resource.getrlimit(resource.RLIMIT_NOFILE) == (128,128)
assert not os.path.exists("/var/run/docker.sock")
assert not os.path.exists("/Users/bytedance/code")
assert not any(k.startswith(("HWOPS_", "AWS_", "DOCKER_")) for k in os.environ)
assert open("/inputs/PROBEINPUT12345678").read() == "private input"
try:
    open("/inputs/PROBEINPUT12345678", "w")
    raise AssertionError("writable input")
except OSError: pass
try:
    open("/root/escape", "w")
    raise AssertionError("writable base")
except OSError: pass
s = socket.socket()
s.settimeout(0.5)
try:
    s.connect(("198.51.100.1",80))
    raise AssertionError("network accessible")
except OSError: pass
finally: s.close()
v=os.statvfs("/work")
assert v.f_blocks*v.f_frsize <= 268435456
print(json.dumps({"numpy":numpy.__version__,"pandas":pandas.__version__,"matplotlib":matplotlib.__version__,"Pillow":PIL.__version__,"PyMuPDF":pymupdf_version,"work_bytes":v.f_blocks*v.f_frsize}))
`

func Probe(ctx context.Context, cfg Config) (Capability, error) {
	if err := verifyEnvironment(ctx, cfg.Image); err != nil {
		return Capability{}, err
	}
	id := rand.Text()
	dir := filepath.Join(cfg.Root, "jobs", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Capability{}, err
	}
	defer os.RemoveAll(dir)
	data := []byte("private input")
	request := Request{ID: id, Code: probeCode, Image: cfg.Image, Budget: domain.PythonLimits(), Deadline: time.Now().Add(time.Minute),
		Inputs: []Input{{ExecutionInput: domain.ExecutionInput{ID: "PROBEINPUT12345678", Kind: "ATTACHMENT", Name: "probe", SHA256: Hash(data), Bytes: int64(len(data))}, Data: data}}}
	if err := stageRequest(dir, request); err != nil {
		return Capability{}, err
	}
	state := execute(dir, request)
	if state.State != "SUCCEEDED" || !state.Result.Cleaned || !state.Result.Complete {
		return Capability{}, fmt.Errorf("probe %s: %s %s", state.State, state.Result.Error, state.Result.Stderr)
	}
	var probe struct {
		Numpy      string `json:"numpy"`
		Pandas     string `json:"pandas"`
		Matplotlib string `json:"matplotlib"`
		Pillow     string `json:"Pillow"`
		PyMuPDF    string `json:"PyMuPDF"`
		WorkBytes  int64  `json:"work_bytes"`
	}
	if err := json.Unmarshal([]byte(state.Result.Stdout), &probe); err != nil ||
		probe.Numpy != "2.2.6" || probe.Pandas != "2.2.3" || probe.Matplotlib != "3.10.3" ||
		probe.Pillow != "11.2.1" || probe.WorkBytes != domain.PythonLimits().WorkBytes {
		return Capability{}, errors.New("execution image packages or workspace quota differ")
	}
	linux, _ := command(ctx, 3*time.Second, "uname", "-sr")
	raw, _ := json.Marshal(state.Result)
	packages := map[string]string{
		"numpy": probe.Numpy, "pandas": probe.Pandas, "matplotlib": probe.Matplotlib, "Pillow": probe.Pillow,
	}
	if probe.PyMuPDF != "" {
		packages["PyMuPDF"] = probe.PyMuPDF
	}
	return Capability{Ready: true, Image: cfg.Image, Runtime: "gVisor 20260928.0", Linux: stringTrim(linux),
		Budget: domain.PythonLimits(), Packages: packages, CheckedAt: time.Now().UTC(), ProbeSHA256: Hash(raw)}, nil
}
