package observability

import (
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
	if endpoint == "" { return func(){}, nil }
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil { return nil, err }
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
	if len(raw) > 64*1024 { return `{"truncated":true}` }
	return string(raw)
}

func Start(ctx context.Context, name, kind string, input any) (context.Context, trace.Span) {
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
	if !sc.IsValid() { return "" }
	return sc.TraceID().String()
}
