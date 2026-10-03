package filestore

import (
	"context"
	"sort"

	"hwops/internal/domain"
)

func (s *Store) CreateIncident(ctx context.Context, in domain.Incident) (out domain.Incident, err error) {
	err = s.transact(ctx, true, func(st *state) error {
		if in.RequestKey != "" {
			for _, old := range st.Incidents {
				if old.RequestKey == in.RequestKey {
					if old.RequestHash != in.RequestHash {
						return domain.ErrConflict
					}
					out = old
					return nil
				}
			}
		}
		if _, exists := st.Incidents[in.ID]; exists {
			return domain.ErrConflict
		}
		st.Incidents[in.ID], out = in, in
		return nil
	})
	return
}

func (s *Store) GetIncident(ctx context.Context, id string) (out domain.Incident, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if out, ok = st.Incidents[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) CreateRun(ctx context.Context, in domain.DiagnosticRun) (out domain.DiagnosticRun, err error) {
	err = s.transact(ctx, true, func(st *state) error {
		if _, ok := st.Incidents[in.IncidentID]; !ok {
			return domain.ErrNotFound
		}
		for _, old := range st.Runs {
			if old.IncidentID == in.IncidentID && !old.Terminal() {
				out = old
				return nil
			}
		}
		if _, exists := st.Runs[in.ID]; exists {
			return domain.ErrConflict
		}
		if st.Devices[in.Device.DeviceID].SnapshotID != in.Device.SnapshotID {
			return domain.ErrConflict
		}
		st.Runs[in.ID], out = in, in
		return nil
	})
	return
}

func (s *Store) GetRun(ctx context.Context, id string) (out domain.DiagnosticRun, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		var ok bool
		if out, ok = st.Runs[id]; !ok {
			return domain.ErrNotFound
		}
		return nil
	})
	return
}

func (s *Store) CommitRun(ctx context.Context, in domain.DiagnosticRun, expected int64, snapshot string, revisions ...string) error {
	return s.transact(ctx, true, func(st *state) error {
		old, ok := st.Runs[in.ID]
		if !ok {
			return domain.ErrNotFound
		}
		if old.StateVersion != expected || in.StateVersion != expected+1 || old.IncidentID != in.IncidentID ||
			(snapshot != "" && st.Devices[in.Device.DeviceID].SnapshotID != snapshot) {
			return domain.ErrConflict
		}
		for _, id := range revisions {
			if st.Revisions[id].Status != "PUBLISHED" {
				return domain.ErrConflict
			}
		}
		st.Runs[in.ID] = in
		return nil
	})
}

func (s *Store) PendingRuns(ctx context.Context) (out []domain.DiagnosticRun, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		for _, r := range st.Runs {
			if r.Pending() {
				out = append(out, r)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
		return nil
	})
	return
}
