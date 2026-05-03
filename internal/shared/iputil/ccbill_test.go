package iputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidCCBillIPRejectsForwardedHeaderLists(t *testing.T) {
	t.Parallel()

	require.True(t, IsValidCCBillIP("64.38.212.1"))
	require.False(t, IsValidCCBillIP("64.38.212.1, 203.0.113.9"))
}
