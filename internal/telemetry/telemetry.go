// Package telemetry configures OpenTelemetry traces, metrics and logs exported over OTLP/HTTP.
//
// All exporter settings come from the standard OTEL_* environment variables
// (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_HEADERS, OTEL_SERVICE_NAME, ...).
// When OTEL_EXPORTER_OTLP_ENDPOINT is unset or OTEL_SDK_DISABLED=true, no-op providers stay in place.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const defaultServiceName = "library"

// Enabled reports whether OTLP export is configured via the environment.
func Enabled() bool {
	return os.Getenv("OTEL_SDK_DISABLED") != "true" && os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}

// Setup installs global tracer, meter and logger providers. Traces go to the OTLP
// backend and/or Langfuse (see LangfuseEnabled); metrics and logs only to OTLP. The returned shutdown
// function flushes and stops all exporters and must be called before exit.
func Setup(ctx context.Context) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	otlpOn, langfuseOn := Enabled(), LangfuseEnabled()
	if !otlpOn && !langfuseOn {
		return noop, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(defaultServiceName)),
	)
	if err != nil {
		return noop, fmt.Errorf("telemetry resource: %w", err)
	}
	// Env-provided attributes (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES) win over the default name.
	envRes, err := resource.New(ctx, resource.WithFromEnv(), resource.WithTelemetrySDK(), resource.WithHost())
	if err != nil {
		return noop, fmt.Errorf("telemetry resource: %w", err)
	}
	if res, err = resource.Merge(res, envRes); err != nil {
		return noop, fmt.Errorf("telemetry resource: %w", err)
	}

	var shutdowns []func(context.Context) error
	shutdown = func(ctx context.Context) error {
		var errs error
		for i := len(shutdowns) - 1; i >= 0; i-- {
			errs = errors.Join(errs, shutdowns[i](ctx))
		}
		return errs
	}
	fail := func(e error) (func(context.Context) error, error) {
		return noop, errors.Join(e, shutdown(ctx))
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	tpOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if otlpOn {
		traceExp, err := otlptracehttp.New(ctx)
		if err != nil {
			return fail(fmt.Errorf("trace exporter: %w", err))
		}
		tpOpts = append(tpOpts, sdktrace.WithBatcher(traceExp))
	}
	if langfuseOn {
		lfExp, err := newLangfuseExporter(ctx)
		if err != nil {
			return fail(fmt.Errorf("langfuse exporter: %w", err))
		}
		tpOpts = append(tpOpts, sdktrace.WithBatcher(lfExp))
	}
	tp := sdktrace.NewTracerProvider(tpOpts...)
	shutdowns = append(shutdowns, tp.Shutdown)
	otel.SetTracerProvider(tp)

	if !otlpOn {
		return shutdown, nil
	}

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return fail(fmt.Errorf("metric exporter: %w", err))
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res),
	)
	shutdowns = append(shutdowns, mp.Shutdown)
	otel.SetMeterProvider(mp)

	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		return fail(fmt.Errorf("log exporter: %w", err))
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)), sdklog.WithResource(res))
	shutdowns = append(shutdowns, lp.Shutdown)
	otel.SetLoggerProvider(lp)

	if err := runtime.Start(); err != nil {
		return fail(fmt.Errorf("runtime metrics: %w", err))
	}

	return shutdown, nil
}

// WrapLogger tees the given logger into the OTel log bridge so every zap entry
// is also exported as an OTLP log record. It is a no-op when telemetry is disabled.
func WrapLogger(logger *zap.Logger) *zap.Logger {
	if !Enabled() {
		return logger
	}
	return logger.WithOptions(zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return zapcore.NewTee(c, otelzap.NewCore("library"))
	}))
}

// Logger returns logger enriched with trace_id/span_id from the active span in ctx.
// The context is attached as a skipped field so the OTel bridge correlates the
// exported record with the span, while console output only shows the ID fields.
func Logger(ctx context.Context, logger *zap.Logger) *zap.Logger {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
		zap.Field{Key: "ctx", Type: zapcore.SkipType, Interface: ctx},
	)
}
