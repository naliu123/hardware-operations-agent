// Package modelbudget reserves every model invocation before dispatch, including
// models nested in retrieval. With no run scope it leaves QA behavior unchanged.
package modelbudget

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type key struct{}

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

type counted struct{ base model.ToolCallingChatModel }

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
	return &counted{base: base}, nil
}

func (m *counted) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if err := Reserve(ctx); err != nil {
		return nil, err
	}
	return m.base.Generate(ctx, messages, opts...)
}

func (m *counted) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if err := Reserve(ctx); err != nil {
		return nil, err
	}
	return m.base.Stream(ctx, messages, opts...)
}
