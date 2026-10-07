// Package modelbudget reserves every model invocation before dispatch, including
// models nested in retrieval. With no run scope it leaves QA behavior unchanged.
package modelbudget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type key struct{}

var ErrExceeded = errors.New("model input or invocation budget exceeded")

type limitKey struct{}

// UTF-8 wire bytes are a conservative token upper estimate for supported
// byte-level tokenizers. Include tools and framing; never silently truncate.
func WithContextLimit(ctx context.Context, inputTokens int) context.Context {
	return context.WithValue(ctx, limitKey{}, inputTokens)
}

func (m *counted) checkInput(ctx context.Context, messages []*schema.Message, opts []model.Option) ([]model.Option, error) {
	limit, ok := ctx.Value(limitKey{}).(int)
	if !ok {
		return opts, nil
	}
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	if len(raw)+m.toolBytes+4096 > limit {
		return nil, ErrExceeded
	}
	options := model.GetCommonOptions(&model.Options{}, opts...)
	if options.MaxTokens == nil || *options.MaxTokens > 8192 {
		opts = append(append([]model.Option{}, opts...), model.WithMaxTokens(8192))
	}
	return opts, nil
}

func WithReservation(ctx context.Context, reserve func(context.Context) error) context.Context {
	return context.WithValue(ctx, key{}, reserve)
}

func Reserve(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if reserve, ok := ctx.Value(key{}).(func(context.Context) error); ok {
		return reserve(ctx)
	}
	return nil
}

type counted struct {
	base      model.ToolCallingChatModel
	toolBytes int
}

func Wrap(base model.BaseChatModel) (model.ToolCallingChatModel, error) {
	caller, ok := base.(model.ToolCallingChatModel)
	if !ok {
		return nil, fmt.Errorf("main agent requires a tool-calling model")
	}
	return &counted{base: caller}, nil
}

func (m *counted) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	base, err := m.base.WithTools(tools)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		return nil, err
	}
	return &counted{base: base, toolBytes: len(raw)}, nil
}

func (m *counted) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts, err := m.checkInput(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	if err := Reserve(ctx); err != nil {
		return nil, err
	}
	return m.base.Generate(ctx, messages, opts...)
}

func (m *counted) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	opts, err := m.checkInput(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	if err := Reserve(ctx); err != nil {
		return nil, err
	}
	return m.base.Stream(ctx, messages, opts...)
}
