// Package telemetry provides OpenTelemetry metrics for the Branch Out service.
package telemetry

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

const (
	// serviceName is the name of the service for telemetry purposes.
	serviceName = "branch-out"
)

// Metrics is a placeholder for telemetry metrics.
type Metrics struct{}

// Options holds the configuration options for the Metrics instance.
type options struct {
	ctx          context.Context
	exporter     string
	otlpEndpoint string
	schemaURL    string
	attributes   []attribute.KeyValue
}

// Option is a function that sets an option for the Metrics instance.
type Option func(*options)

// WithContext sets the context for the Metrics instance.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

// WithExporter sets the exporter for the Metrics instance.
func WithExporter(exporter string) Option {
	return func(o *options) {
		o.exporter = exporter
	}
}

// WithOTLPEndpoint sets the OTLP endpoint for the Metrics instance.
func WithOTLPEndpoint(endpoint string) Option {
	return func(o *options) {
		o.otlpEndpoint = endpoint
	}
}

// WithAttributes sets the attributes for the Metrics instance.
func WithAttributes(attributes ...attribute.KeyValue) Option {
	return func(o *options) {
		o.attributes = append(o.attributes, attributes...)
	}
}

// WithSchemaURL sets the schema URL for the Metrics instance.
func WithSchemaURL(schemaURL string) Option {
	return func(o *options) {
		o.schemaURL = schemaURL
	}
}

// Attribute is a helper function to create an attribute.KeyValue.
func Attribute(key string, value any) attribute.KeyValue {
	return attribute.String(key, value.(string))
}

// defaultOptions returns the default options for the Metrics instance.
func defaultOptions() *options {
	return &options{
		exporter:     "stdout",
		otlpEndpoint: "localhost:4317",
		schemaURL:    "https://opentelemetry.io/schemas/1.4.0",
		attributes:   []attribute.KeyValue{Attribute("service.name", serviceName)},
	}
}

// NewMetrics creates a new instance of Metrics.
// It sets up the OpenTelemetry SDK for metrics collection and returns a shutdown function.
//
//	if shutdown != nil {
//		// Register the shutdown function to be called when the application exits.
//		defer func() {
//			if err := shutdown(opts.ctx); err != nil {
//				panic("failed to shut down OpenTelemetry SDK: " + err.Error())
//			}
//		}()
//	}
func NewMetrics(options ...Option) (*Metrics, func(context.Context) error, error) {
	opts := defaultOptions()
	for _, opt := range options {
		opt(opts)
	}
	m := &Metrics{}
	shutdown, err := m.setupOtelSDK(opts.ctx, opts)
	if err != nil {
		return nil, nil, errors.New("failed to set up OpenTelemetry SDK: " + err.Error())
	}
	return m, shutdown, nil
}

// setupOtelSDK initializes the OpenTelemetry SDK for metrics collection.
func (m *Metrics) setupOtelSDK(ctx context.Context, opts *options) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := m.newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up meter provider.
	meterProvider, err := m.newMeterProvider(opts)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	return
}

// newPropagator creates a new OpenTelemetry text map propagator.
// It combines TraceContext and Baggage propagators.
// This allows for distributed tracing and baggage propagation across service boundaries.
func (m *Metrics) newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// newMeterProvider creates a new OpenTelemetry meter provider.
// This is a placeholder function and should be implemented to return a valid meter provider.
func (m *Metrics) newMeterProvider(opts *options) (*metric.MeterProvider, error) {
	var (
		metricExporter metric.Exporter
		err            error
	)
	switch opts.exporter {
	case "stdout":
		metricExporter, err = stdoutmetric.New(stdoutmetric.WithPrettyPrint())
		if err != nil {
			return nil, err
		}
	case "otlp":
		metricExporter, err = otlpmetricgrpc.New(opts.ctx,
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithEndpoint(opts.otlpEndpoint),
		)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported exporter type: " + opts.exporter)
	}

	res, err := resource.New(
		opts.ctx,
		resource.WithAttributes(opts.attributes...),
		resource.WithSchemaURL("https://opentelemetry.io/schemas/1.4.0"),
	)
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
	)
	return meterProvider, nil
}
