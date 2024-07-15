package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hashicorp/memberlist"
	"github.com/misalcedo/gedcb"
	"log"
	"math"
	"net"
	"time"
)

type GossipState struct {
	Age   int
	State gedcb.State
}

type ClusterDelegate struct {
	name          string
	state         map[string]GossipState
	breaker       *gedcb.Breaker
	clusterConfig *memberlist.Config
	cluster       *memberlist.Memberlist
	queue         *memberlist.TransmitLimitedQueue
}

func (c *ClusterDelegate) FastJoin(cluster string) error {
	peers, err := c.fetchPeers(cluster)
	if err != nil {
		return fmt.Errorf("failed to join the cluster: %w", err)
	}

	if len(peers) == 0 {
		return errors.New("no peers")
	}

	_, err = c.cluster.Join(peers[0:1])
	if err != nil {
		return fmt.Errorf("failed to join the cluster: %w", err)
	}

	return nil
}

func (c *ClusterDelegate) Join(ctx context.Context, cluster string) error {
	var n int
	var err error
	var peers []string

	start := time.Now()

OuterLoop:
	for {
		members := c.cluster.NumMembers()
		if members > 1 {
			log.Printf("successfully joined %d nodes after %f seconds\n", members, time.Since(start).Seconds())
			break OuterLoop
		}

		select {
		case <-ctx.Done():
			return err
		default:
			peers, err = c.fetchPeers(cluster)
			if err != nil {
				log.Println("failed to join the cluster", err)
				continue
			}

			n, err = c.cluster.Join(peers)
			if err != nil {
				log.Println("failed to join cluster", err)
				continue
			}

			log.Printf("successfully joined %d nodes after %f seconds\n", n, time.Since(start).Seconds())
			break OuterLoop
		}
	}

	return nil
}

func (c *ClusterDelegate) fetchPeers(cluster string) ([]string, error) {
	addresses, err := net.LookupIP(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster domain name: %w", err)
	}

	addr := c.cluster.LocalNode().Addr
	peers := make([]string, 0, len(addresses))

	for _, peer := range addresses {
		if peer.Equal(addr) {
			continue
		}

		peers = append(peers, peer.String())
	}

	log.Println("joining cluster with peers", peers)

	return peers, nil
}

func (c *ClusterDelegate) NotifyJoin(node *memberlist.Node) {
	c.state[node.Name] = GossipState{
		// Set to the max age so a new update will override this.
		Age:   c.maxAge(),
		State: gedcb.Closed,
	}
}

func (c *ClusterDelegate) NotifyLeave(node *memberlist.Node) {
	delete(c.state, node.Name)
}

func (c *ClusterDelegate) NotifyUpdate(*memberlist.Node) {
}

func (c *ClusterDelegate) NodeMeta(int) []byte {
	return nil
}

func (c *ClusterDelegate) NotifyMsg(msg []byte) {
	log.Println("received message", string(msg))
}

func (c *ClusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return c.queue.GetBroadcasts(overhead, limit)
}

func (c *ClusterDelegate) LocalState(join bool) []byte {
	log.Printf("LocalState join: %v\n", join)

	// increment the age of all the local state.
	for name, state := range c.state {
		c.state[name] = GossipState{
			Age:   state.Age + 1,
			State: state.State,
		}
	}

	c.state[c.name] = GossipState{
		Age:   0,
		State: c.breaker.State(time.Now()),
	}

	bytes, err := json.Marshal(c.state)
	if err != nil {
		log.Println("failed to marshal local state", err)
	}

	return bytes
}

func (c *ClusterDelegate) MergeRemoteState(buf []byte, join bool) {
	var remoteState map[string]GossipState

	err := json.Unmarshal(buf, &remoteState)
	if err != nil {
		log.Println("failed to unmarshal local state", err)
	}

	log.Printf("MergeRemoteState join: %v, state: %v\n", join, remoteState)

	for name, state := range remoteState {
		if state.Age < state.Age {
			log.Printf("Updated state for %s: %v->%v\n", name, c.state[name].State, state.State)
			c.state[name] = state
		}
	}
}

func (c *ClusterDelegate) maxAge() int {
	members := 1

	if c.cluster != nil {
		members = c.cluster.NumMembers()
	}

	return int(math.Ceil(float64(c.clusterConfig.SuspicionMult) * math.Log(float64(members+1))))
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
		name:          clusterConfig.Name,
		breaker:       breaker,
		state:         make(map[string]GossipState),
		clusterConfig: clusterConfig,
	}
	clusterConfig.Delegate = delegate
	clusterConfig.Events = delegate

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
