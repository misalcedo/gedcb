package main

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/memberlist"
	"github.com/misalcedo/gedcb"
	"log"
	"math/rand"
	"net"
	"sync/atomic"
	"time"
)

type ClusterDelegate struct {
	name          string
	dirty         atomic.Bool
	breaker       *gedcb.Breaker
	version       int
	peerVersions  map[string]int
	clusterConfig *memberlist.Config
	cluster       *memberlist.Memberlist
	queue         *memberlist.TransmitLimitedQueue
}

func (c *ClusterDelegate) NotifyJoin(*memberlist.Node) {
}

func (c *ClusterDelegate) NotifyLeave(node *memberlist.Node) {
	c.breaker.DeletePeer(node.Name)
}

func (c *ClusterDelegate) NotifyUpdate(*memberlist.Node) {
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

	rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	n, err := c.cluster.Join(peers[0:1])
	if err != nil {
		return err
	}

	log.Printf("successfully connected %d nodes after %f seconds\n", n, time.Since(start).Seconds())

	return nil
}

func (c *ClusterDelegate) fetchPeers(cluster string, peerAddresses []string) ([]string, error) {
	if cluster == "" || (cluster == "localhost" && len(peerAddresses) > 0) {
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

func (c *ClusterDelegate) NodeMeta(int) []byte {
	return nil
}

func (c *ClusterDelegate) NotifyMsg(msg []byte) {
	var stateBroadcast CircuitBreakerBroadcast

	err := json.Unmarshal(msg, &stateBroadcast)
	if err != nil {
		log.Println("failed to unmarshal broadcast", err)
	}

	peerVersion, found := c.peerVersions[stateBroadcast.Name]
	if !found || stateBroadcast.Version > peerVersion {
		log.Printf("updated state for %s to %v via broadcast\n", stateBroadcast.Name, stateBroadcast.State)
		c.breaker.UpdatePeer(stateBroadcast.Name, stateBroadcast.State)
	} else {
		log.Printf("ignoring outdated state for %s\n", stateBroadcast.Name)
	}
}

func (c *ClusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	if c.dirty.Swap(false) {
		c.version++
		c.queue.QueueBroadcast(CircuitBreakerBroadcast{
			Name:    c.name,
			Version: c.version,
			State:   c.breaker.State(time.Now()),
		})
	}

	return c.queue.GetBroadcasts(overhead, limit)
}

func (c *ClusterDelegate) LocalState(bool) []byte {
	return nil
}

func (c *ClusterDelegate) MergeRemoteState([]byte, bool) {
}

func NewBreakerDelegate(clusterConfig *memberlist.Config) (*ClusterDelegate, error) {
	delegate := &ClusterDelegate{
		name:          clusterConfig.Name,
		clusterConfig: clusterConfig,
		peerVersions:  make(map[string]int),
	}

	breakerConfig := gedcb.BreakerConfig{
		WindowSize:                time.Minute,
		SuspicionSuccessThreshold: 10,
		SoftFailureThreshold:      5,
		HardFailureThreshold:      50,
		HalfOpenFailureThreshold:  2,
		HalfOpenSuccessThreshold:  2,
		OpenDuration:              time.Second * 1,
		OnStateChange: func(_, newState gedcb.State) {
			delegate.dirty.Store(true)
		},
	}

	decay := gedcb.NewDecay(time.Now(), gedcb.ExponentialDecayFunction(0.1, breakerConfig.WindowSize))
	delegate.breaker = gedcb.NewBreaker(breakerConfig, decay)
	delegate.dirty.Store(true)

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
