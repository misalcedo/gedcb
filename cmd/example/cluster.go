package main

import (
	"encoding/json"
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

func (c *ClusterDelegate) Join(cluster string, peerAddresses []string) error {
	start := time.Now()

	peers, err := c.fetchPeers(cluster, peerAddresses)
	if err != nil {
		return err
	}

	if len(peers) == 0 {
		log.Println("no peers to join")
		return nil
	}

	log.Printf("attempting to join %v nodes from %s to the cluster with %d members\n", peers, c.cluster.LocalNode().Address(), c.cluster.NumMembers())

	n, err := c.cluster.Join(peers)
	if err != nil {
		return err
	}

	log.Printf("successfully connected %d nodes after %f seconds\n", n, time.Since(start).Seconds())

	return nil
}

func (c *ClusterDelegate) fetchPeers(cluster string, peerAddresses []string) ([]string, error) {
	if cluster == "localhost" && len(peerAddresses) > 0 {
		return c.filterPeers(peerAddresses), nil
	}

	ipAddresses, err := net.LookupIP(cluster)
	if err != nil {
		return nil, err
	}

	addresses := make([]string, 0, len(ipAddresses))
	for _, peer := range ipAddresses {
		addresses = append(addresses, fmt.Sprintf("%s:%d", peer.String(), c.clusterConfig.BindPort))
	}

	return c.filterPeers(addresses), nil
}

func (c *ClusterDelegate) filterPeers(peers []string) []string {
	filtered := make([]string, 0, len(peers))

OuterLoop:
	for _, peer := range peers {
		for _, node := range c.cluster.Members() {
			if node.Address() == peer {
				continue OuterLoop
			}
		}

		filtered = append(filtered, peer)
	}

	return filtered
}

func (c *ClusterDelegate) NotifyMerge(peers []*memberlist.Node) error {
	log.Printf("%s is merging state from %v\n", c.name, peers)
	return nil
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

func (c *ClusterDelegate) NotifyMsg([]byte) {
}

func (c *ClusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return c.queue.GetBroadcasts(overhead, limit)
}

func (c *ClusterDelegate) LocalState(join bool) []byte {
	if !join {
		// increment the age of all the local state.
		for name, state := range c.state {
			c.state[name] = GossipState{
				Age:   state.Age + 1,
				State: state.State,
			}
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
	clusterConfig.Merge = delegate

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
