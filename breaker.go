package gedcb

import (
	"errors"
	"math"
	"sync"
	"time"
)

type BreakerConfig struct {
	WindowSize                time.Duration
	SuspicionSuccessThreshold int
	SoftFailureThreshold      int
	HardFailureThreshold      int
	HalfOpenFailureThreshold  int
	HalfOpenSuccessThreshold  int
	OpenDuration              time.Duration
	OnStateChange             func(State, State)
}

type Breaker struct {
	config          BreakerConfig
	decay           ForwardDecay
	state           State
	successes       float64
	failures        float64
	deadline        time.Time
	peers           map[string]State
	majoritySuspect bool
	mutex           sync.Mutex
}

type State int

const (
	Closed State = iota
	Suspicion
	Open
	HalfOpen
)

// OpenBreakerErr is returned when the breaker is open.
var OpenBreakerErr = errors.New("open breaker")

// NewBreaker creates a new breaker with the given configuration, decay function, and landmark.
func NewBreaker(config BreakerConfig, decay ForwardDecay) *Breaker {
	return &Breaker{
		config:   config,
		decay:    decay,
		state:    Closed,
		deadline: decay.Landmark(),
		peers:    make(map[string]State),
	}
}

// Acquire returns an error if the breaker is open. Otherwise, it returns nil.
func (b *Breaker) Acquire(timestamp time.Time) error {
	if b.State(timestamp) == Open {
		return OpenBreakerErr
	}

	return nil
}

// Success records a success in the breaker. It returns an error if the breaker is open.
func (b *Breaker) Success(timestamp time.Time) error {
	if b.state == Open {
		return OpenBreakerErr
	}

	item := NewBasicItem(time.Now(), 1.0)
	b.successes += b.decay.StaticWeight(item)
	b.Transition(timestamp)

	return nil
}

// Failure records a failure in the breaker. It returns an error if the breaker is open.
func (b *Breaker) Failure(timestamp time.Time) error {
	if b.state == Open {
		return OpenBreakerErr
	}

	item := NewBasicItem(timestamp, 1.0)
	b.failures += b.decay.StaticWeight(item)
	b.Transition(timestamp)

	return nil
}

// Transition computes the new state of the breaker based on the current state and the number of successes and failures.
func (b *Breaker) Transition(timestamp time.Time) {
	initialState := b.state

	switch initialState {
	case Closed:
		if b.Failures(timestamp) > b.config.SoftFailureThreshold {
			b.state = Suspicion
		}
	case Suspicion:
		if b.Successes(timestamp) > b.config.SuspicionSuccessThreshold {
			b.state = Closed
			b.clearWindow()
		} else if b.Failures(timestamp) > b.config.HardFailureThreshold {
			b.state = Open
			b.clearWindow()
			b.startTimer(timestamp)
		} else if b.majoritySuspect {
			b.state = Open
			b.clearWindow()
			b.startTimer(timestamp)
		}
	case Open:
		if timestamp.After(b.deadline) {
			b.state = HalfOpen
		}
	case HalfOpen:
		if b.Failures(timestamp) > b.config.HalfOpenFailureThreshold {
			b.state = Open
			b.clearWindow()
			b.startTimer(timestamp)
		} else if b.Successes(timestamp) > b.config.HalfOpenSuccessThreshold {
			b.state = Closed
			b.clearWindow()
		}
	}

	if b.state != initialState && b.config.OnStateChange != nil {
		b.config.OnStateChange(initialState, b.state)
	}
}

// Successes returns the number of successes in the breaker's current window.
func (b *Breaker) Successes(timestamp time.Time) int {
	return int(math.Ceil(b.successes / b.decay.NormalizingFactor(timestamp)))
}

// Failures returns the number of failures in the breaker's current window.
func (b *Breaker) Failures(timestamp time.Time) int {
	return int(math.Ceil(b.failures / b.decay.NormalizingFactor(timestamp)))
}

// clearWindow resets the number of successes and failures in the breaker's current window.
// It also resets the window's deadline, used as the timer for transitioning from Open to HalfOpen.
func (b *Breaker) clearWindow() {
	b.successes = 0
	b.failures = 0
	b.deadline = b.decay.Landmark()
}

// startTimer sets the deadline for the breaker to transition from Open to HalfOpen.
func (b *Breaker) startTimer(timestamp time.Time) {
	b.deadline = timestamp.Add(b.config.OpenDuration)
}

// Deadline returns the deadline for the breaker to transition from Open to HalfOpen.
func (b *Breaker) Deadline() time.Time {
	return b.deadline
}

// State returns the current state of the breaker. It also updates the state based on the current time.
func (b *Breaker) State(timestamp time.Time) State {
	age := b.decay.SetLandmark(timestamp)
	factor := b.decay.G(age)

	b.successes /= factor
	b.failures /= factor
	b.Transition(timestamp)

	return b.state
}

// UpdatePeer updates the state of a peer in the breaker. Then, recomputes whether the majority of peers suspect a failure.
// This can be called concurrently from any go-routine.
func (b *Breaker) UpdatePeer(peer string, state State) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.peers[peer] = state
	b.majoritySuspect = b.computeMajoritySuspect()
}

// DeletePeer removes the state of a peer in the breaker. Then, recomputes whether the majority of peers suspect a failure.
// This can be called concurrently from any go-routine.
func (b *Breaker) DeletePeer(peer string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	delete(b.peers, peer)
	b.majoritySuspect = b.computeMajoritySuspect()
}

// computeMajoritySuspect returns true if the majority of peers suspect a failure.
func (b *Breaker) computeMajoritySuspect() bool {
	total := 0
	majority := len(b.peers)/2 + 1

	for _, peer := range b.peers {
		if peer != Closed {
			total++
		}
	}

	return total >= majority
}
