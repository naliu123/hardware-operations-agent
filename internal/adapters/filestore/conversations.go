package filestore

import (
	"context"
	"errors"

	"hwops/internal/domain"
)

func findRequest(st *state, conversationID, key, hash string) (domain.Response, error) {
	if key != "" {
		for _, r := range st.Responses {
			if r.ConversationID == conversationID && r.RequestKey == key {
				if r.RequestHash != hash {
					return domain.Response{}, domain.ErrConflict
				}
				return r, nil
			}
		}
	}
	return domain.Response{}, domain.ErrNotFound
}

func (s *Store) FindResponseRequest(ctx context.Context, conversationID, key, hash string) (r domain.Response, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		r, err = findRequest(st, conversationID, key, hash)
		return err
	})
	return
}

func (s *Store) CreateResponse(ctx context.Context, r domain.Response, expectedVersion int64) (out domain.Response, err error) {
	err = s.transact(ctx, true, func(st *state) error {
		existing, err := findRequest(st, r.ConversationID, r.RequestKey, r.RequestHash)
		if err == nil {
			out = existing
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		c, ok := st.Conversations[r.ConversationID]
		if !ok {
			return domain.ErrNotFound
		}
		if expectedVersion >= 0 && c.ContextVersion != expectedVersion {
			return domain.ErrConflict
		}
		if _, exists := st.Responses[r.ID]; exists {
			return domain.ErrConflict
		}
		c.ApplyResponseContext(r)
		st.Conversations[c.ID] = c
		saveResponse(st, r)
		out = st.Responses[r.ID]
		return nil
	})
	return
}

func (s *Store) ResponseEvents(ctx context.Context, id string, after int64) (out []domain.ResponseEvent, err error) {
	err = s.transact(ctx, false, func(st *state) error {
		if _, exists := st.Responses[id]; !exists {
			return domain.ErrNotFound
		}
		out = []domain.ResponseEvent{}
		for _, event := range st.Events[id] {
			if event.ID > after {
				out = append(out, event)
			}
		}
		return nil
	})
	return
}
