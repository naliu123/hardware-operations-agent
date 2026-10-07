// Package execution owns durable Python identities, bounded dispatch and
// reconciliation. A disconnected caller cannot restart an execution.
package execution

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"hwops/internal/blobstore"
	"hwops/internal/domain"
	"hwops/internal/sandbox"
)

type Repository interface {
	domain.PythonRepository
	GetResponse(context.Context, string) (domain.Response, error)
}

type InputResolver func(context.Context, string, []string) ([]sandbox.Input, error)

type Service struct {
	repo    Repository
	runner  sandbox.Runner
	files   *blobstore.Store
	cap     sandbox.Capability
	resolve InputResolver
	ctx     context.Context
	cancel  context.CancelFunc
	wake    chan struct{}
	wg      sync.WaitGroup
}

func New(repo Repository, runner sandbox.Runner, files *blobstore.Store, resolve InputResolver) (*Service, error) {
	if runner == nil || files == nil {
		return nil, sandbox.ErrUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	probeCtx, probeCancel := context.WithTimeout(ctx, 20*time.Second)
	cap, err := runner.Capability(probeCtx)
	probeCancel()
	if err != nil || !cap.Ready || cap.Budget != domain.PythonLimits() || cap.Runtime == "" {
		cancel()
		return nil, sandbox.ErrUnavailable
	}
	for name, version := range map[string]string{
		"numpy": "2.2.6", "pandas": "2.2.3", "matplotlib": "3.10.3", "Pillow": "11.2.1",
	} {
		if cap.Packages[name] != version {
			cancel()
			return nil, sandbox.ErrUnavailable
		}
	}
	s := &Service{repo: repo, runner: runner, files: files, cap: cap, resolve: resolve, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	if err = repo.InterruptPythonExecutions(ctx); err != nil {
		cancel()
		return nil, err
	}
	s.wg.Add(1)
	go s.work()
	return s, nil
}

func (s *Service) Close() { s.cancel(); s.wg.Wait() }
func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) inputs(ctx context.Context, cid string, ids []string) ([]sandbox.Input, error) {
	out := []sandbox.Input{}
	var attachments []string
	var total int64
	seen := map[string]bool{}
	if len(ids) > 20 {
		return nil, domain.ErrInvalid
	}
	for _, id := range ids {
		if seen[id] {
			return nil, domain.ErrInvalid
		}
		seen[id] = true
		a, err := s.repo.GetArtifact(ctx, id)
		if errors.Is(err, domain.ErrNotFound) && s.resolve != nil {
			attachments = append(attachments, id)
			continue
		}
		if err != nil {
			return nil, err
		}
		if a.ConversationID != cid {
			return nil, domain.ErrNotFound
		}
		total += a.Bytes
		if total > 100_000_000 {
			return nil, domain.ErrInvalid
		}
		f, err := s.files.Open(a.StorageKey)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, a.Bytes+1))
		f.Close()
		if err != nil || int64(len(data)) != a.Bytes || sandbox.Hash(data) != a.SHA256 {
			return nil, errors.New("authorized input content mismatch")
		}
		out = append(out, sandbox.Input{ExecutionInput: domain.ExecutionInput{ID: a.ID, Kind: "ARTIFACT", Name: a.Name, SHA256: a.SHA256, Bytes: a.Bytes}, Data: data})
	}
	if len(attachments) > 0 {
		extra, err := s.resolve(ctx, cid, attachments)
		if err != nil {
			return nil, err
		}
		out = append(out, extra...)
	}
	return out, nil
}

func (s *Service) Start(ctx context.Context, responseID, code string, inputIDs []string) (domain.PythonExecution, error) {
	r, err := s.repo.GetResponse(ctx, responseID)
	if err != nil {
		return domain.PythonExecution{}, err
	}
	inputs, err := s.inputs(ctx, r.ConversationID, inputIDs)
	if err != nil {
		return domain.PythonExecution{}, err
	}
	request := sandbox.Request{ID: rand.Text(), Code: code, Inputs: inputs, Image: s.cap.Image, Budget: domain.PythonLimits(), Deadline: r.Deadline}
	if err = request.Validate(); err != nil {
		return domain.PythonExecution{}, domain.ErrInvalid
	}
	e := domain.PythonExecution{ID: request.ID, OwnerID: domain.Owner(ctx), ConversationID: r.ConversationID, ResponseID: r.ID,
		Code: code, CodeSHA256: sandbox.Hash([]byte(code)), RequestSHA256: request.Hash(), Image: request.Image, Budget: request.Budget,
		Deadline: r.Deadline, CreatedAt: time.Now().UTC(), Status: "QUEUED", Inputs: []domain.ExecutionInput{}}
	for _, input := range inputs {
		e.Inputs = append(e.Inputs, input.ExecutionInput)
	}
	e, err = s.repo.CreatePythonExecution(ctx, e)
	if err == nil {
		s.notify()
	}
	return e, err
}

func (s *Service) Wait(ctx context.Context, id string) (domain.PythonExecution, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		e, err := s.repo.GetPythonExecution(ctx, id)
		if err != nil {
			return e, err
		}
		if domain.PythonTerminal(e.Status) && e.Result.Cleaned {
			return e, nil
		}
		select {
		case <-ctx.Done():
			// Persist cancellation independently from the timed-out model call.
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			reason := "CANCELED"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				reason = "DEADLINE_EXCEEDED"
			}
			_ = s.repo.CancelPythonExecution(cleanup, id, reason)
			cancel()
			s.notify()
			return e, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) Get(ctx context.Context, id string) (domain.PythonExecution, error) {
	return s.repo.GetPythonExecution(ctx, id)
}

func (s *Service) Artifact(ctx context.Context, id string) (domain.Artifact, *os.File, error) {
	a, err := s.repo.GetArtifact(ctx, id)
	if err != nil {
		return a, nil, err
	}
	f, err := s.files.Open(a.StorageKey)
	return a, f, err
}

func (s *Service) work() {
	defer s.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending, err := s.repo.PendingPythonExecutions(s.ctx)
		if err == nil {
			occupied := 0
			for _, e := range pending {
				if e.Status != "QUEUED" {
					occupied++
					s.reconcile(e)
				}
			}
			for _, e := range pending {
				if e.Status != "QUEUED" {
					continue
				}
				if !time.Now().Before(e.Deadline) {
					ctx := domain.WithUser(s.ctx, domain.User{ID: e.OwnerID})
					_ = s.repo.CancelPythonExecution(ctx, e.ID, "DEADLINE_EXCEEDED")
					continue
				}
				if occupied >= 2 {
					continue
				}
				occupied++
				s.dispatch(e)
			}
			s.cleanupDeleted()
		}
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Service) dispatch(e domain.PythonExecution) {
	version := e.Version
	e.Status = "STARTING"
	if s.repo.SavePythonExecution(s.ctx, e, version) != nil {
		return
	}
	e.Version++
	ctx := domain.WithUser(s.ctx, domain.User{ID: e.OwnerID})
	ids := []string{}
	for _, input := range e.Inputs {
		ids = append(ids, input.ID)
	}
	inputs, err := s.inputs(ctx, e.ConversationID, ids)
	if err != nil {
		_ = s.repo.CancelPythonExecution(ctx, e.ID, "CANCELED")
		return
	}
	request := sandbox.Request{ID: e.ID, Code: e.Code, Inputs: inputs, Image: e.Image, Budget: e.Budget, Deadline: e.Deadline}
	if request.Hash() != e.RequestSHA256 || request.Validate() != nil {
		_ = s.repo.CancelPythonExecution(ctx, e.ID, "CANCELED")
		return
	}
	state, err := s.runner.Submit(s.ctx, request)
	if err != nil {
		// This request is never automatically submitted again. GET/CANCEL
		// reconcile the same ID even when the dispatch reply was lost.
		return
	}
	s.accept(e, state)
}

func (s *Service) reconcile(e domain.PythonExecution) {
	ctx := domain.WithUser(s.ctx, domain.User{ID: e.OwnerID})
	if e.CancelReason == "" && !time.Now().Before(e.Deadline) {
		_ = s.repo.CancelPythonExecution(ctx, e.ID, "DEADLINE_EXCEEDED")
		return
	}
	var state sandbox.Status
	var err error
	if e.CancelReason != "" {
		state, err = s.runner.Cancel(s.ctx, e.ID)
	} else {
		state, err = s.runner.Get(s.ctx, e.ID)
		if errors.Is(err, sandbox.ErrMissing) {
			// A cancel tombstone closes the race with delayed network delivery.
			_ = s.repo.CancelPythonExecution(ctx, e.ID, "INTERRUPTED")
			return
		}
	}
	if err == nil {
		s.accept(e, state)
	}
}

func (s *Service) accept(e domain.PythonExecution, state sandbox.Status) {
	if state.ID != e.ID || (state.RequestSHA256 != e.RequestSHA256 && !(e.CancelReason != "" && state.RequestSHA256 == "")) {
		return
	}
	if state.State != "STARTING" && state.State != "RUNNING" && !domain.PythonTerminal(state.State) {
		return
	}
	if domain.PythonTerminal(state.State) && !state.Result.Cleaned {
		return
	}
	version := e.Version
	e.Status, e.Result = state.State, state.Result
	if e.CancelReason != "" && state.Result.Cleaned {
		e.Status = "CANCELED"
		if e.CancelReason == "INTERRUPTED" {
			e.Status = "INTERRUPTED"
		}
		if e.CancelReason == "DEADLINE_EXCEEDED" {
			e.Status = "FAILED"
		}
		e.Result.Error, e.Result.Complete, e.Result.Artifacts = e.CancelReason, false, nil
	}
	staged := []string{}
	if e.Status == "SUCCEEDED" {
		if !e.Result.Complete || e.Result.ExitCode == nil || *e.Result.ExitCode != 0 ||
			len(e.Result.Artifacts) > 20 || len(e.Result.Stdout)+len(e.Result.Stderr) > 1<<20 {
			return
		}
		var total int64
		for i := range e.Result.Artifacts {
			a := &e.Result.Artifacts[i]
			total += a.Bytes
			if a.ExecutionID != e.ID || a.Bytes < 0 || total > 100_000_000 {
				s.discard(staged)
				return
			}
			stream, err := s.runner.Artifact(s.ctx, e.ID, a.ID)
			if err != nil {
				s.discard(staged)
				return
			}
			n, _, err := s.files.Put(a.ID, stream, a.Bytes, a.SHA256)
			stream.Close()
			if err != nil || n != a.Bytes {
				s.discard(staged)
				return
			}
			staged = append(staged, a.ID)
			a.StorageKey, a.ConversationID = a.ID, e.ConversationID
		}
	}
	if e.Status == state.State && (e.Status == "STARTING" || e.Status == "RUNNING") {
		// Avoid changing the version for repeated polling of identical state.
		saved, err := s.repo.GetPythonExecution(domain.WithUser(s.ctx, domain.User{ID: e.OwnerID}), e.ID)
		if err == nil && saved.Status == e.Status {
			return
		}
	}
	if s.repo.SavePythonExecution(s.ctx, e, version) != nil {
		s.discard(staged)
	}
}

func (s *Service) discard(keys []string) {
	for _, key := range keys {
		_ = s.files.Delete(key)
	}
}

func (s *Service) cleanupDeleted() {
	executions, err := s.repo.DeletedPythonExecutions(s.ctx)
	if err != nil {
		return
	}
	for _, e := range executions {
		if !domain.PythonTerminal(e.Status) || !e.Result.Cleaned {
			continue
		}
		if s.runner.Forget(s.ctx, e.ID) != nil {
			continue
		}
		clean := true
		for _, a := range e.Result.Artifacts {
			if s.files.Delete(a.ID) != nil {
				clean = false
			}
		}
		if clean {
			_ = s.repo.DeletePythonExecution(s.ctx, e.ID)
		}
	}
}
