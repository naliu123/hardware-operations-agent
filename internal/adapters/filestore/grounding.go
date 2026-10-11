package filestore

import (
	"context"

	"hwops/internal/domain"
)

func (s *Store) SaveGroundingReview(ctx context.Context, id string, review domain.GroundingReview, usage *domain.ModelUsage) error {
	return s.transact(ctx, true, func(st *state) error {
		r, ok := st.Responses[id]
		if !ok {
			return domain.ErrNotFound
		}
		if r.Status != "RUNNING" {
			return domain.ErrConflict
		}
		r.GroundingReviews = append(r.GroundingReviews, review)
		r.ModelUsage = usage
		st.Responses[id] = r
		return nil
	})
}
