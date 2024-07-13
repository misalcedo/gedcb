package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"time"
)

func main() {
	var requests int
	var availability float64

	flag.IntVar(&requests, "requests", 100, "Number of requests")
	flag.Float64Var(&availability, "availability", 1.0, "Probability of a given request succeeding")
	flag.Parse()

	config := BreakerConfig{
		WindowSize:                time.Minute,
		SuspicionSuccessThreshold: 10,
		SoftFailureThreshold:      5,
		HardFailureThreshold:      50,
		HalfOpenFailureThreshold:  2,
		HalfOpenSuccessThreshold:  2,
		OpenDuration:              time.Second * 1,
	}
	breaker := NewBreaker(config, 0.1, time.Now())

	rejected := 0

	for i := 0; i < requests; i++ {
		now := time.Now()

		if rand.Float64() < availability {
			if err := breaker.Success(now); err != nil {
				rejected++
			}
		} else {
			if err := breaker.Failure(now); err != nil {
				rejected++
			}
		}

		switch breaker.State(now) {
		case Closed:
		case Suspicion:
			fmt.Printf("Breaker in Suspicion state with %d successes and %d failures\n", breaker.Successes(now), breaker.Failures(now))
		case Open:
			fmt.Printf("Breaker in Open state with %s remaining\n", -time.Since(breaker.Deadline()))
			time.Sleep(50 * time.Millisecond)
		case HalfOpen:
			fmt.Printf("Breaker in HalfOpen state with %d successes and %d failures\n", breaker.Successes(now), breaker.Failures(now))
		}
	}

	fmt.Printf("Rejected %d requests\n", rejected)
}
