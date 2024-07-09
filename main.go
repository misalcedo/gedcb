package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"time"
)

// Usage go run ./... 1000000 0.6
func main() {
	if len(os.Args) < 3 {
		panic("usage: go run main.go 1000000 0.25")
	}

	requests, err := strconv.Atoi(os.Args[1])
	if err != nil {
		panic("Invalid number of requests")
	}

	failureProbability, err := strconv.ParseFloat(os.Args[2], 64)
	if err != nil {
		panic("Invalid failure probability")
	}

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

		if rand.Float64() >= failureProbability {
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
