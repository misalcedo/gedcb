package main

import (
	"encoding/json"
	"github.com/hashicorp/memberlist"
	"github.com/misalcedo/gedcb"
	"log"
)

type CircuitBreakerBroadcast struct {
	Name  string
	State gedcb.State
}

func (c CircuitBreakerBroadcast) Invalidates(b memberlist.Broadcast) bool {
	if old, ok := b.(CircuitBreakerBroadcast); ok {
		return c.Name == old.Name
	}

	return false
}

func (c CircuitBreakerBroadcast) Message() []byte {
	bytes, err := json.Marshal(&c)
	if err != nil {
		log.Println("failed to marshal broadcast", err)
	}

	return bytes
}

func (c CircuitBreakerBroadcast) Finished() {
}
