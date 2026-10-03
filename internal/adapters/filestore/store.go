// Package filestore implements a single-process development repository.
package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"

	"hwops/internal/domain"
)

type state struct {
	VersionPolicies map[string]domain.VersionPolicy   `json:"version_policies"`
	Devices         map[string]domain.DeviceContext   `json:"devices"`
	Revisions       map[string]domain.Revision        `json:"revisions"`
	Conversations   map[string]domain.Conversation    `json:"conversations"`
	Responses       map[string]domain.Response        `json:"responses"`
	Events          map[string][]domain.ResponseEvent `json:"response_events"`
}

type Store struct {
	mu   sync.Mutex
	path string
	lock *os.File
	data state
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("repository already open: %w", err)
	}
	s := &Store{path: path, lock: lock, data: state{
		Revisions: map[string]domain.Revision{}, Conversations: map[string]domain.Conversation{},
		Responses: map[string]domain.Response{},
		Devices:   map[string]domain.DeviceContext{},
	}}
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &s.data)
		if err == nil && (s.data.Revisions == nil || s.data.Conversations == nil || s.data.Responses == nil) {
			err = errors.New("invalid repository state")
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Close()
		return nil, err
	}
	if s.data.Devices == nil {
		s.data.Devices = map[string]domain.DeviceContext{}
	}
	if s.data.VersionPolicies == nil {
		s.data.VersionPolicies = map[string]domain.VersionPolicy{}
	}
	if s.data.Events == nil {
		s.data.Events = map[string][]domain.ResponseEvent{}
	}
	return s, nil
}

func (s *Store) CreateVersionPolicy(ctx context.Context, p domain.VersionPolicy) error {
	return s.transact(ctx, true, func(st *state) error {
		if _, exists := st.VersionPolicies[p.ID]; exists {
			return domain.ErrConflict
		}
		st.VersionPolicies[p.ID] = p
		return nil
	})
}

func (s *Store) GetVersionPolicy(ctx context.Context, id string) (p domain.VersionPolicy, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if p, ok = st.VersionPolicies[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) PutDevice(ctx context.Context, d domain.DeviceContext) error {
	return s.transact(ctx, true, func(st *state) error {
		if old, ok := st.Devices[d.DeviceID]; ok && !d.ObservedAt.After(old.ObservedAt) {
			return fmt.Errorf("%w: device observation must be newer than the saved snapshot", domain.ErrConflict)
		}
		st.Devices[d.DeviceID] = d
		return nil
	})
}

func (s *Store) GetDevice(ctx context.Context, id string) (d domain.DeviceContext, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if d, ok = st.Devices[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) ListDevices(ctx context.Context) (out []domain.DeviceContext, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		for _, d := range st.Devices {
			out = append(out, d)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
		return nil
	})
	return
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

// transact copies state before modification and publishes it only after rename.
func (s *Store) transact(ctx context.Context, write bool, fn func(*state) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.lock == nil {
		return errors.New("repository closed")
	}
	raw, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	var next state
	if err := json.Unmarshal(raw, &next); err != nil {
		return err
	}
	if err := fn(&next); err != nil || !write {
		return err
	}
	raw, err = json.Marshal(next)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.data = next
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Store) CreateRevision(ctx context.Context, r domain.Revision) error {
	return s.transact(ctx, true, func(st *state) error { st.Revisions[r.ID] = r; return nil })
}

func (s *Store) GetRevision(ctx context.Context, id string) (r domain.Revision, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if r, ok = st.Revisions[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) SetPublication(ctx context.Context, id, status string) (r domain.Revision, err error) {
	err = s.transact(ctx, true, func(st *state) error {
		var ok bool
		if r, ok = st.Revisions[id]; !ok {
			return domain.ErrNotFound
		}
		r.Status = status
		st.Revisions[id] = r
		return nil
	})
	return
}

func (s *Store) ListPublished(ctx context.Context) (out []domain.Revision, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		for _, r := range st.Revisions {
			if r.Status == "PUBLISHED" {
				out = append(out, r)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return nil
	})
	return
}

func (s *Store) CreateConversation(ctx context.Context, c domain.Conversation) error {
	return s.transact(ctx, true, func(st *state) error { st.Conversations[c.ID] = c; return nil })
}

func (s *Store) GetConversation(ctx context.Context, id string) (c domain.Conversation, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if c, ok = st.Conversations[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) SaveResponse(ctx context.Context, r domain.Response) error {
	return s.transact(ctx, true, func(st *state) error {
		saveResponse(st, r)
		return nil
	})
}

func saveResponse(st *state, r domain.Response) {
	if r.DeviceContext != nil {
		r.CheckDeviceSnapshot(st.Devices[r.DeviceContext.DeviceID].SnapshotID)
	}
	events := st.Events[r.ID]
	st.Events[r.ID] = append(events, domain.NewResponseEvents(st.Responses[r.ID].Status, r, int64(len(events)))...)
	st.Responses[r.ID] = r
}

func (s *Store) GetResponse(ctx context.Context, id string) (r domain.Response, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if r, ok = st.Responses[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) PendingResponses(ctx context.Context) (out []domain.Response, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		for _, r := range st.Responses {
			if r.Status == "QUEUED" || r.Status == "RUNNING" {
				out = append(out, r)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
		return nil
	})
	return
}
