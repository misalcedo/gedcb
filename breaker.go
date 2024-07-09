// https://aashaypalliwar.github.io/assets/pdf/gedcb-main.pdf
package main

import (
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
}

type Breaker struct {
	config    BreakerConfig
	decay     ForwardDecay
	state     State
	successes float64
	failures  float64
	deadline  time.Time
	peers     map[any]State
	mutex     sync.Mutex
}

type State int

const (
	Closed State = iota
	Suspicion
	Open
	HalfOpen
)

func NewBreaker(config BreakerConfig, landmark time.Time) *Breaker {
	return &Breaker{
		config:   config,
		decay:    NewDecay(landmark, ExponentialDecayFunction(0.001, config.WindowSize)),
		state:    Closed,
		deadline: landmark,
	}
}

func (b *Breaker) Success() {
	item := NewBasicItem(time.Now(), 1.0)
	b.successes += b.decay.StaticWeight(item)
}

func (b *Breaker) Failure() {
	now := time.Now()
	item := NewBasicItem(now, 1.0)

	b.failures += b.decay.StaticWeight(item)
	b.Transition(now)
}

func (b *Breaker) Transition(timestamp time.Time) {
	switch b.state {
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
		} else if b.majoritySuspecting() {
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
}

func (b *Breaker) Successes(timestamp time.Time) int {
	return int(math.Ceil(b.successes / b.decay.NormalizingFactor(timestamp)))
}

func (b *Breaker) Failures(timestamp time.Time) int {
	return int(math.Ceil(b.failures / b.decay.NormalizingFactor(timestamp)))
}

func (b *Breaker) clearWindow() {
	b.successes = 0
	b.failures = 0
	b.deadline = b.decay.Landmark()
}

func (b *Breaker) startTimer(timestamp time.Time) {
	b.deadline = timestamp.Add(b.config.OpenDuration)
}

func (b *Breaker) majoritySuspecting() bool {
	total := 0
	majority := len(b.peers)/2 + 1

	for _, peer := range b.peers {
		if peer != Closed {
			total++
		}
	}

	return total >= majority
}

func (b *Breaker) State(timestamp time.Time) State {
	age := b.decay.SetLandmark(timestamp)
	factor := b.decay.G(age)

	b.successes /= factor
	b.failures /= factor
	b.Transition(timestamp)

	return b.state
}

func (b *Breaker) UpdatePeers(peers map[any]State) {
	for key := range b.peers {
		if _, ok := peers[key]; !ok {
			delete(b.peers, key)
		}
	}

	for key, value := range peers {
		b.peers[key] = value
	}
}
