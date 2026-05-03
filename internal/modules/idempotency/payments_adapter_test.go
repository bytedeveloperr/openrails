package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPaymentsIdempotencyAdapterCompleteStoresBytesAsRawJSON(t *testing.T) {
	ctx := context.Background()
	svc := NewIdempotencyServiceWithTTL(nil, time.Minute)
	adapter := NewPaymentsIdempotencyAdapter(svc)

	require.NoError(t, adapter.Complete(ctx, "checkout", "key", []byte(`{"id":"cs_123"}`)))

	rec, err := svc.Get(ctx, "checkout", "key")
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.JSONEq(t, `{"id":"cs_123"}`, string(rec.Result))
}
