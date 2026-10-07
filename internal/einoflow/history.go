package einoflow

import (
	"context"

	"hwops/internal/domain"
)

type historyKey struct{}

func WithHistory(ctx context.Context, history *domain.ConversationHistory) context.Context {
	return context.WithValue(ctx, historyKey{}, history)
}

func historyFrom(ctx context.Context) *domain.ConversationHistory {
	history, _ := ctx.Value(historyKey{}).(*domain.ConversationHistory)
	return history
}
