package gedcb

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBreaker(t *testing.T) {
	landmark := time.Now()
	config := BreakerConfig{
		WindowSize:                time.Minute,
		SuspicionSuccessThreshold: 10,
		SoftFailureThreshold:      5,
		HardFailureThreshold:      50,
		HalfOpenFailureThreshold:  2,
		HalfOpenSuccessThreshold:  2,
		OpenDuration:              time.Second * 1,
	}
	decay := NewDecay(landmark, ExponentialDecayFunction(0.1, config.WindowSize))
	breaker := NewBreaker(config, decay)

	require.NoError(t, breaker.Acquire(landmark))
	require.NoError(t, breaker.Success(landmark))

	// fail just short of suspicion
	for i := 0; i < config.SoftFailureThreshold; i++ {
		require.NoError(t, breaker.Failure(landmark))
	}
	require.Equal(t, Closed, breaker.State(landmark))
	require.NoError(t, breaker.Failure(landmark))
	require.Equal(t, Suspicion, breaker.State(landmark))

	// succeed just short of closed
	// window is not reset on suspicion
	for i := 0; i < config.SuspicionSuccessThreshold-1; i++ {
		require.NoError(t, breaker.Success(landmark))
	}
	require.Equal(t, Suspicion, breaker.State(landmark))
	require.NoError(t, breaker.Success(landmark))
	require.Equal(t, Closed, breaker.State(landmark))

	for i := 0; i < config.HardFailureThreshold+1; i++ {
		require.NoError(t, breaker.Failure(landmark))
	}
	require.EqualError(t, breaker.Acquire(landmark), OpenBreakerErr.Error())

	for i := 100 * time.Millisecond; i < time.Second; i += 100 * time.Millisecond {
		require.EqualError(t, breaker.Acquire(landmark.Add(i)), OpenBreakerErr.Error())
	}

	now := landmark.Add(time.Second + time.Millisecond)
	require.Equal(t, HalfOpen, breaker.State(now))

	for i := 0; i < config.HalfOpenFailureThreshold+1; i++ {
		require.NoError(t, breaker.Failure(now))
	}
	require.Equal(t, Open, breaker.State(now))

	now = now.Add(time.Second + time.Millisecond)
	require.Equal(t, HalfOpen, breaker.State(now))

	for i := 0; i < config.HalfOpenSuccessThreshold+1; i++ {
		require.NoError(t, breaker.Success(now))
	}
	require.Equal(t, Closed, breaker.State(now))
}
