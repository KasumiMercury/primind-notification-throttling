//go:build !loadtest

package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

func appendLoadtestLabels(_ context.Context, attrs []attribute.KeyValue) []attribute.KeyValue {
	return attrs
}
