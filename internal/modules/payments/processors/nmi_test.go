package processors

import (
	"testing"

	"github.com/open-rails/openrails/internal/db/models"
	"github.com/stretchr/testify/require"
)

func TestSameProcessor(t *testing.T) {
	t.Parallel()

	require.True(t, SameProcessor(models.Processor(" Mobius "), models.ProcessorMobius))
	require.False(t, SameProcessor(models.ProcessorMobius, models.Processor("other_nmi")))
	require.False(t, SameProcessor(models.Processor(""), models.ProcessorMobius))
}
