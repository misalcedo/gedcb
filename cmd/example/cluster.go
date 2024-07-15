package main

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/memberlist"
	"github.com/misalcedo/gedcb"
	"log"
	"net"
	"sort"
	"time"
)

type ClusterDelegate struct {
	name    string
	state   map[string]gedcb.State
	breaker *gedcb.Breaker
	cluster *memberlist.Memberlist
	queue   *memberlist.TransmitLimitedQueue
}

// Join an existing cluster by specifying at least one known member.
func (c *ClusterDelegate) Join(peers []net.IP) error {
	if len(peers) == 0 {
		return fmt.Errorf("peers must not be empty")
	}

	// Deterministically choose coordinators
	coordinators := make([]string, 0, 2)

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].String() < peers[j].String()
	})

	if len(peers) > 0 {
		coordinators = append(coordinators, peers[0].String())
	}

	if len(peers) > 1 {
		coordinators = append(coordinators, peers[len(peers)-1].String())
	}

	n, err := c.cluster.Join([]string{peers[0].String()})
	if err == nil {
		log.Printf("successfully joined %d nodes\n", n)
	} else {
		log.Println("failed to join cluster", err)
	}

	return nil
}

func (c *ClusterDelegate) NodeMeta(limit int) []byte {
	return nil
}

func (c *ClusterDelegate) NotifyMsg(msg []byte) {
	fmt.Printf("msg: %s\n", string(msg))
}

func (c *ClusterDelegate) GetBroadcasts(overhead int, limit int) [][]byte {
	return nil
}

func (c *ClusterDelegate) LocalState(join bool) []byte {
	fmt.Printf("LocalState join: %v\n", join)

	c.state[c.name] = c.breaker.State(time.Now())

	bytes, err := json.Marshal(c.state)
	if err != nil {
		panic(err)
	}

	return bytes
}

func (c *ClusterDelegate) MergeRemoteState(buf []byte, join bool) {
	var state map[string]gedcb.State

	err := json.Unmarshal(buf, &state)
	if err != nil {
		panic(err)
	}

	fmt.Printf("MergeRemoteState join: %v, state: %v\n", join, state)
}

func NewBreakerDelegate(clusterConfig *memberlist.Config) (*ClusterDelegate, error) {
	breakerConfig := gedcb.BreakerConfig{
		WindowSize:                time.Minute,
		SuspicionSuccessThreshold: 10,
		SoftFailureThreshold:      5,
		HardFailureThreshold:      50,
		HalfOpenFailureThreshold:  2,
		HalfOpenSuccessThreshold:  2,
		OpenDuration:              time.Second * 1,
	}
	breaker := gedcb.NewBreaker(breakerConfig, 0.1, time.Now())

	delegate := &ClusterDelegate{
		name:    clusterConfig.Name,
		breaker: breaker,
		state:   make(map[string]gedcb.State),
	}
	clusterConfig.Delegate = delegate

	cluster, err := memberlist.Create(clusterConfig)
	if err != nil {
		return nil, err
	}

	delegate.cluster = cluster
	delegate.queue = &memberlist.TransmitLimitedQueue{
		NumNodes:       cluster.NumMembers,
		RetransmitMult: clusterConfig.RetransmitMult,
	}

	return delegate, nil
}
