package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/scoutpulse/libs/platform/server"
)

// OTLPEndpointEnvVar configures the trace collector. Tracing is disabled when
// it is unset.
const OTLPEndpointEnvVar = "OTEL_EXPORTER_OTLP_ENDPOINT"

// Shutdown flushes buffered telemetry.
type Shutdown func(context.Context) error

// InitTracing configures the global tracer provider from the environment.
//
// Tracing is optional by design: a service must start and serve requests with
// no collector reachable, because telemetry is diagnostic and cannot be a
// runtime dependency of handling a request. With no endpoint configured this
// installs a no-op and returns.
func InitTracing(ctx context.Context, serviceName, version string, logger *slog.Logger) (Shutdown, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Cross-service context propagation is installed regardless, so a trace
	// id set by an upstream caller is carried even when this process is not
	// exporting.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := os.Getenv(OTLPEndpointEnvVar)
	if endpoint == "" {
		logger.Info("no " + OTLPEndpointEnvVar + " set, tracing is disabled")
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("environment", os.Getenv("ENVIRONMENT")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("building trace resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		// Parent-based sampling keeps a trace whole: a request sampled at the
		// edge stays sampled through every service it touches.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(samplingRatio()))),
	)
	otel.SetTracerProvider(provider)

	logger.Info("tracing enabled", "endpoint", endpoint, "service", serviceName)

	return func(shutdownCtx context.Context) error {
		return provider.Shutdown(shutdownCtx)
	}, nil
}

// samplingRatio reads OTEL_TRACES_SAMPLER_ARG, defaulting to sampling
// everything. A production deployment should lower it.
func samplingRatio() float64 {
	raw := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if raw == "" {
		return 1.0
	}

	var ratio float64
	if _, err := fmt.Sscanf(raw, "%f", &ratio); err != nil || ratio < 0 || ratio > 1 {
		return 1.0
	}
	return ratio
}

// Tracing returns middleware that opens a span per request and continues any
// trace the caller propagated.
func Tracing(serviceName string) server.Middleware {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, serviceName,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				// Name spans by route pattern, for the same cardinality
				// reason the metrics labels use it.
				if r.Pattern != "" {
					return r.Method + " " + r.Pattern
				}
				return r.Method
			}),
			// The scrape endpoint would otherwise produce a span on every
			// scrape interval, forever.
			otelhttp.WithFilter(func(r *http.Request) bool {
				return r.URL.Path != MetricsPath && r.URL.Path != "/health"
			}),
		)
	}
}

// SpanContextFrom returns the active span context, for correlating a log line
// with a trace.
func SpanContextFrom(ctx context.Context) trace.SpanContext {
	return trace.SpanContextFromContext(ctx)
}
