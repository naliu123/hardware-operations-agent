package einoflow

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const maxStreamMessageBytes = 1024 * 1024

type DraftEmitter interface {
	BeginDraft(context.Context) error
	EmitDraftDelta(context.Context, string) error
}

type draftEmitterKey struct{}

type ReasoningEmitter interface {
	EmitReasoningDelta(context.Context, string) error
	FinishReasoning(context.Context) error
}

type reasoningEmitterKey struct{}

func WithReasoningEmitter(ctx context.Context, emitter ReasoningEmitter) context.Context {
	return context.WithValue(ctx, reasoningEmitterKey{}, emitter)
}

func WithDraftEmitter(ctx context.Context, emitter DraftEmitter) context.Context {
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, draftEmitterKey{}, emitter)
}

func streamGenerate(ctx context.Context, cm model.BaseChatModel, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := cm.Stream(callCtx, input, opts...)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var chunks []*schema.Message
	var bytes int
	extractor := newVisibleTextExtractor()
	emitter, _ := ctx.Value(draftEmitterKey{}).(DraftEmitter)
	reasoning, _ := ctx.Value(reasoningEmitterKey{}).(ReasoningEmitter)
	thinking := false
	finishReasoning := func() error {
		if thinking && reasoning != nil {
			if err := reasoning.FinishReasoning(ctx); err != nil {
				return err
			}
			thinking = false
		}
		return nil
	}
	began := false
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			return nil, ErrInvalidAnswer
		}
		bytes += len(chunk.Content) + len(chunk.ReasoningContent)
		for _, call := range chunk.ToolCalls {
			bytes += len(call.ID) + len(call.Type) + len(call.Function.Name) + len(call.Function.Arguments)
		}
		if bytes > maxStreamMessageBytes {
			return nil, ErrInvalidAnswer
		}
		if chunk.ReasoningContent != "" && reasoning != nil {
			if err = reasoning.EmitReasoningDelta(ctx, chunk.ReasoningContent); err != nil {
				return nil, err
			}
			thinking = true
		}
		if chunk.Content != "" || len(chunk.ToolCalls) > 0 {
			if err = finishReasoning(); err != nil {
				return nil, err
			}
		}
		if visible := extractor.Write(chunk.Content); visible != "" && emitter != nil {
			if !began {
				if err = emitter.BeginDraft(ctx); err != nil {
					return nil, err
				}
				began = true
			}
			if err = emitter.EmitDraftDelta(ctx, visible); err != nil {
				return nil, err
			}
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		return nil, ErrInvalidAnswer
	}
	if err = finishReasoning(); err != nil {
		return nil, err
	}
	return schema.ConcatMessages(chunks)
}

type jsonFrame struct {
	kind      byte
	role      string
	key       string
	expectKey bool
}

// visibleTextExtractor recognizes only root claims[].text and reply.text JSON
// strings. It never emits keys, tool arguments, reasoning, gaps, or other fields.
type visibleTextExtractor struct {
	stack        []jsonFrame
	inString     bool
	stringIsKey  bool
	stringTarget bool
	escape       bool
	unicodeLeft  int
	unicodeValue rune
	pendingHigh  rune
	decoded      strings.Builder
	targetCount  int
	targetOpen   bool
	poisoned     bool
}

func newVisibleTextExtractor() *visibleTextExtractor {
	return &visibleTextExtractor{}
}

func (e *visibleTextExtractor) Write(input string) string {
	if e.poisoned || input == "" {
		return ""
	}
	var out strings.Builder
	for i := 0; i < len(input); i++ {
		b := input[i]
		if e.inString {
			e.consumeStringByte(b, &out)
			if e.poisoned {
				return out.String()
			}
			continue
		}
		switch b {
		case '"':
			e.startString()
		case '{':
			role := "object"
			if len(e.stack) == 0 {
				role = "root"
			} else if parent := &e.stack[len(e.stack)-1]; parent.kind == '[' && parent.role == "claims" {
				role = "claim"
			} else if parent.kind == '{' {
				if parent.role == "root" && parent.key == "reply" {
					role = "reply"
				}
				parent.key = ""
			}
			e.stack = append(e.stack, jsonFrame{kind: '{', role: role, expectKey: true})
		case '[':
			role := "array"
			if len(e.stack) > 0 {
				parent := &e.stack[len(e.stack)-1]
				if parent.kind == '{' && parent.role == "root" && parent.key == "claims" {
					role = "claims"
				}
				if parent.kind == '{' {
					parent.key = ""
				}
			}
			e.stack = append(e.stack, jsonFrame{kind: '[', role: role})
		case '}', ']':
			if len(e.stack) == 0 || (b == '}' && e.stack[len(e.stack)-1].kind != '{') ||
				(b == ']' && e.stack[len(e.stack)-1].kind != '[') {
				e.poisoned = true
				return out.String()
			}
			e.stack = e.stack[:len(e.stack)-1]
		case ',':
			if len(e.stack) > 0 && e.stack[len(e.stack)-1].kind == '{' {
				frame := &e.stack[len(e.stack)-1]
				frame.key, frame.expectKey = "", true
			}
		}
	}
	return out.String()
}

func (e *visibleTextExtractor) startString() {
	e.inString = true
	e.escape = false
	e.unicodeLeft = 0
	e.unicodeValue = 0
	e.pendingHigh = 0
	e.decoded.Reset()
	e.stringIsKey, e.stringTarget, e.targetOpen = false, false, false
	if len(e.stack) == 0 {
		return
	}
	frame := &e.stack[len(e.stack)-1]
	e.stringIsKey = frame.kind == '{' && frame.expectKey
	e.stringTarget = frame.kind == '{' && (frame.role == "claim" || frame.role == "reply") &&
		frame.key == "text" && !e.stringIsKey
}

func (e *visibleTextExtractor) consumeStringByte(b byte, out *strings.Builder) {
	if e.unicodeLeft > 0 {
		value, ok := hexValue(b)
		if !ok {
			e.poisoned = true
			return
		}
		e.unicodeValue = e.unicodeValue<<4 | rune(value)
		e.unicodeLeft--
		if e.unicodeLeft == 0 {
			e.finishUnicode(out)
		}
		return
	}
	if e.escape {
		e.escape = false
		if e.pendingHigh != 0 && b != 'u' {
			e.poisoned = true
			return
		}
		switch b {
		case '"', '\\', '/':
			e.emitString(string([]byte{b}), out)
		case 'b':
			e.emitString("\b", out)
		case 'f':
			e.emitString("\f", out)
		case 'n':
			e.emitString("\n", out)
		case 'r':
			e.emitString("\r", out)
		case 't':
			e.emitString("\t", out)
		case 'u':
			e.unicodeLeft, e.unicodeValue = 4, 0
		default:
			e.poisoned = true
		}
		return
	}
	if e.pendingHigh != 0 && b != '\\' {
		e.poisoned = true
		return
	}
	switch b {
	case '\\':
		e.escape = true
	case '"':
		if e.pendingHigh != 0 {
			e.poisoned = true
			return
		}
		e.finishString()
	default:
		if b < 0x20 {
			e.poisoned = true
			return
		}
		e.emitString(string([]byte{b}), out)
	}
}

func (e *visibleTextExtractor) finishUnicode(out *strings.Builder) {
	value := e.unicodeValue
	switch {
	case value >= 0xd800 && value <= 0xdbff:
		if e.pendingHigh != 0 {
			e.poisoned = true
			return
		}
		e.pendingHigh = value
	case value >= 0xdc00 && value <= 0xdfff:
		if e.pendingHigh == 0 {
			e.poisoned = true
			return
		}
		e.emitString(string(utf16.DecodeRune(e.pendingHigh, value)), out)
		e.pendingHigh = 0
	case e.pendingHigh != 0:
		e.poisoned = true
	default:
		e.emitString(string(value), out)
	}
}

func (e *visibleTextExtractor) emitString(value string, out *strings.Builder) {
	if e.stringIsKey {
		e.decoded.WriteString(value)
	}
	if !e.stringTarget {
		return
	}
	if !e.targetOpen {
		if e.targetCount > 0 {
			out.WriteString("\n\n")
		}
		e.targetCount++
		e.targetOpen = true
	}
	out.WriteString(value)
}

func (e *visibleTextExtractor) finishString() {
	if len(e.stack) > 0 {
		frame := &e.stack[len(e.stack)-1]
		if e.stringIsKey {
			frame.key, frame.expectKey = e.decoded.String(), false
		} else if frame.kind == '{' {
			frame.key = ""
		}
	}
	e.inString, e.stringIsKey, e.stringTarget, e.targetOpen = false, false, false, false
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
