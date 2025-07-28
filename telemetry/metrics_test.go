package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestNewMetrics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		options []Option
		wantErr bool
	}{
		{
			name:    "default options",
			options: nil,
			wantErr: false,
		},
		{
			name: "with context",
			options: []Option{
				WithContext(context.Background()),
			},
			wantErr: false,
		},
		{
			name: "with stdout exporter",
			options: []Option{
				WithExporter("stdout"),
			},
			wantErr: false,
		},
		{
			name: "with custom attributes",
			options: []Option{
				WithAttributes(
					attribute.String("test.key", "test.value"),
					attribute.String("environment", "test"),
				),
			},
			wantErr: false,
		},
		{
			name: "with schema URL",
			options: []Option{
				WithSchemaURL("https://opentelemetry.io/schemas/1.4.0"),
			},
			wantErr: false,
		},
		{
			name: "unsupported exporter",
			options: []Option{
				WithExporter("unsupported"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			metrics, shutdown, err := NewMetrics(tt.options...)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, metrics)
				assert.Nil(t, shutdown)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, metrics)
			require.NotNil(t, shutdown)

			// Test shutdown function
			err = shutdown(context.Background())
			require.NoError(t, err)

			// Test that shutdown can be called multiple times safely
			err = shutdown(context.Background())
			assert.NoError(t, err)
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()
	opts := defaultOptions()

	assert.Equal(t, "stdout", opts.exporter)
	assert.Equal(t, "localhost:4317", opts.otlpEndpoint)
	assert.Equal(t, "https://opentelemetry.io/schemas/1.4.0", opts.schemaURL)
	assert.Len(t, opts.attributes, 1)
	assert.Equal(t, "service.name", string(opts.attributes[0].Key))
	assert.Equal(t, serviceName, opts.attributes[0].Value.AsString())
}

func TestWithContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	opts := &options{}

	WithContext(ctx)(opts)

	assert.Equal(t, ctx, opts.ctx)
}

func TestWithExporter(t *testing.T) {
	t.Parallel()
	opts := &options{}

	WithExporter("otlp")(opts)

	assert.Equal(t, "otlp", opts.exporter)
}

func TestWithOTLPEndpoint(t *testing.T) {
	t.Parallel()
	opts := &options{}

	WithOTLPEndpoint("localhost:8080")(opts)

	assert.Equal(t, "localhost:8080", opts.otlpEndpoint)
}

func TestWithAttributes(t *testing.T) {
	t.Parallel()
	opts := &options{}
	attr1 := attribute.String("key1", "value1")
	attr2 := attribute.String("key2", "value2")

	WithAttributes(attr1, attr2)(opts)

	assert.Len(t, opts.attributes, 2)
	assert.Equal(t, attr1, opts.attributes[0])
	assert.Equal(t, attr2, opts.attributes[1])
}

func TestWithSchemaURL(t *testing.T) {
	t.Parallel()
	opts := &options{}
	schemaURL := "https://example.com/schema"

	WithSchemaURL(schemaURL)(opts)

	assert.Equal(t, schemaURL, opts.schemaURL)
}

func TestAttribute(t *testing.T) {
	t.Parallel()
	attr := Attribute("test.key", "test.value")

	assert.Equal(t, "test.key", string(attr.Key))
	assert.Equal(t, "test.value", attr.Value.AsString())
}

func TestMetricsNewPropagator(t *testing.T) {
	t.Parallel()
	m := &Metrics{}

	propagator := m.newPropagator()

	assert.NotNil(t, propagator)

	// Test that the propagator has the expected fields
	fields := propagator.Fields()
	expectedFields := []string{"traceparent", "tracestate", "baggage"}

	for _, expected := range expectedFields {
		assert.Contains(t, fields, expected)
	}
}

func TestMetricsNewMeterProvider(t *testing.T) {
	t.Parallel()
	m := &Metrics{}
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    *options
		wantErr bool
	}{
		{
			name: "stdout exporter",
			opts: &options{
				ctx:          ctx,
				exporter:     "stdout",
				otlpEndpoint: "localhost:4317",
				schemaURL:    "https://opentelemetry.io/schemas/1.4.0",
				attributes:   []attribute.KeyValue{attribute.String("service.name", serviceName)},
			},
			wantErr: false,
		},
		{
			name: "unsupported exporter",
			opts: &options{
				ctx:          ctx,
				exporter:     "unsupported",
				otlpEndpoint: "localhost:4317",
				schemaURL:    "https://opentelemetry.io/schemas/1.4.0",
				attributes:   []attribute.KeyValue{attribute.String("service.name", serviceName)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider, err := m.newMeterProvider(tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, provider)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)

			// Test shutdown
			err = provider.Shutdown(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestSetupOtelSDK(t *testing.T) {
	t.Parallel()
	m := &Metrics{}
	ctx := context.Background()
	opts := defaultOptions()
	opts.ctx = ctx

	shutdown, err := m.setupOtelSDK(ctx, opts)

	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// Test shutdown
	err = shutdown(ctx)
	assert.NoError(t, err)
}

func TestServiceName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "branch-out", serviceName)
}
