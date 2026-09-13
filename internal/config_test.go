package heartbeats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	heartbeats "github.com/siutsin/heartbeats/internal"
)

// TestDefaultConfig verifies production default values.
func TestDefaultConfig(t *testing.T) {

	config := heartbeats.DefaultConfig()

	require.Equal(t, 10*time.Second, config.DefaultTimeout)
	require.Equal(t, 3, config.MaxRetries)
	require.Equal(t, 1*time.Second, config.RetryDelay)
	require.Equal(t, 5*time.Second, config.RequeueAfter)
}
