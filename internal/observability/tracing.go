package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Configure is optional. Only explicitly enabled deployments export knowledge
// and question text to their configured trusted Phoenix collector.
func Configure(ctx context.Context, endpoint, project string) (func(), error) {
	if endpoint == "" {
		return func() {}, nil
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "hwopsd"),
			attribute.String("openinference.project.name", project))))
	otel.SetTracerProvider(provider)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	}, nil
}

func JSON(value any) string {
	raw, _ := json.Marshal(value)
	// Provider reasoning can occur in nested model messages/tool loops. Never
	// export it, even when question/answer tracing is explicitly enabled.
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&tree) == nil {
		omitReasoning(tree)
		raw, _ = json.Marshal(tree)
	}
	if len(raw) > 64*1024 {
		return `{"truncated":true}`
	}
	return string(raw)
}

func omitReasoning(value any) {
	switch node := value.(type) {
	case map[string]any:
		delete(node, "reasoning")
		delete(node, "reasoning_content")
		for _, child := range node {
			omitReasoning(child)
		}
	case []any:
		for _, child := range node {
			omitReasoning(child)
		}
	}
}

type privateKey struct{}

func WithoutContent(ctx context.Context) context.Context {
	return context.WithValue(ctx, privateKey{}, true)
}

type privateSpan struct{ trace.Span }

func (s privateSpan) SetAttributes(attrs ...attribute.KeyValue) {
	filtered := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		switch string(attr.Key) {
		case "openinference.span.kind", "session.id", "llm.model_name", "llm.provider", "llm.system",
			"llm.retry_count", "llm.token_count.prompt", "llm.token_count.completion", "llm.token_count.total",
			"embedding.model_name":
			filtered = append(filtered, attr)
		}
	}
	s.Span.SetAttributes(filtered...)
}

func Start(ctx context.Context, name, kind string, input any) (context.Context, trace.Span) {
	if ctx.Value(privateKey{}) == true {
		ctx, span := otel.Tracer("hwops").Start(ctx, name, trace.WithAttributes(attribute.String("openinference.span.kind", kind)))
		private := privateSpan{span}
		return trace.ContextWithSpan(ctx, private), private
	}
	return otel.Tracer("hwops").Start(ctx, name, trace.WithAttributes(
		attribute.String("openinference.span.kind", kind),
		attribute.String("input.mime_type", "application/json"),
		attribute.String("input.value", JSON(input))))
}

func End(span trace.Span, output any, err error) {
	if err != nil {
		// Never copy arbitrary upstream errors (which may contain credentials).
		span.SetStatus(codes.Error, "operation failed")
	} else {
		span.SetStatus(codes.Ok, "")
		span.SetAttributes(attribute.String("output.mime_type", "application/json"),
			attribute.String("output.value", JSON(output)))
	}
	span.End()
}

func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
