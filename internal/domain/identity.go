package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("forbidden")
	ErrRateLimited  = errors.New("too many login attempts")
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

type ownerKey struct{}
type roleKey struct{}

func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(WithOwner(ctx, user.ID), roleKey{}, user.Role)
}

func IsAdmin(ctx context.Context) bool { return ctx.Value(roleKey{}) == "ADMIN" }

// WithOwner is set by trusted authentication code, never request JSON or a model.
// An absent owner is reserved for the isolated local_token runtime and workers.
func WithOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerKey{}, owner)
}

func Owner(ctx context.Context) string {
	owner, _ := ctx.Value(ownerKey{}).(string)
	return owner
}
