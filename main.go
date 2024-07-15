package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hashicorp/memberlist"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//func main() {
//	var requests int
//	var availability float64
//
//	flag.IntVar(&requests, "requests", 100, "Number of requests")
//	flag.Float64Var(&availability, "availability", 1.0, "Probability of a given request succeeding")
//	flag.Parse()
//
//	rejected := 0
//
//	for i := 0; i < requests; i++ {
//		now := time.Now()
//
//		if rand.Float64() < availability {
//			if err := breaker.Success(now); err != nil {
//				rejected++
//			}
//		} else {
//			if err := breaker.Failure(now); err != nil {
//				rejected++
//			}
//		}
//
//		switch breaker.State(now) {
//		case Closed:
//		case Suspicion:
//			fmt.Printf("Breaker in Suspicion state with %d successes and %d failures\n", breaker.Successes(now), breaker.Failures(now))
//		case Open:
//			fmt.Printf("Breaker in Open state with %s remaining\n", -time.Since(breaker.Deadline()))
//			time.Sleep(50 * time.Millisecond)
//		case HalfOpen:
//			fmt.Printf("Breaker in HalfOpen state with %d successes and %d failures\n", breaker.Successes(now), breaker.Failures(now))
//		}
//	}
//
//	fmt.Printf("Rejected %d requests\n", rejected)
//}

type GossipState struct {
	Age   int   `json:"age"`
	State State `json:"state"`
}

type BreakerDelegate struct {
	name    string
	state   map[string]GossipState
	breaker *Breaker
	cluster *memberlist.Memberlist
	queue   *memberlist.TransmitLimitedQueue
}

// Join an existing cluster by specifying at least one known member.
func (b *BreakerDelegate) Join(peers []string) {
	n, err := b.cluster.Join(peers)
	if err == nil {
		log.Printf("successfully joined %d nodes\n", n)
	} else {
		log.Println("failed to join cluster", err)
	}
}

func (b *BreakerDelegate) NodeMeta(limit int) []byte {
	return nil
}

func (b *BreakerDelegate) NotifyMsg(msg []byte) {
	fmt.Printf("msg: %s\n", string(msg))
}

func (b *BreakerDelegate) GetBroadcasts(overhead int, limit int) [][]byte {
	return nil
}

func (b *BreakerDelegate) LocalState(join bool) []byte {
	fmt.Printf("LocalState join: %v\n", join)

	b.state[b.name] = GossipState{
		Age:   0,
		State: b.breaker.state,
	}

	bytes, err := json.Marshal(b.state)
	if err != nil {
		panic(err)
	}

	return bytes
}

func (b *BreakerDelegate) MergeRemoteState(buf []byte, join bool) {
	var state map[string]GossipState

	err := json.Unmarshal(buf, &state)
	if err != nil {
		panic(err)
	}

	fmt.Printf("MergeRemoteState join: %v, state: %v\n", join, state)
}

func NewBreakerDelegate(clusterConfig *memberlist.Config) (*BreakerDelegate, error) {
	breakerConfig := BreakerConfig{
		WindowSize:                time.Minute,
		SuspicionSuccessThreshold: 10,
		SoftFailureThreshold:      5,
		HardFailureThreshold:      50,
		HalfOpenFailureThreshold:  2,
		HalfOpenSuccessThreshold:  2,
		OpenDuration:              time.Second * 1,
	}
	breaker := NewBreaker(breakerConfig, 0.1, time.Now())

	delegate := &BreakerDelegate{
		name:    clusterConfig.Name,
		breaker: breaker,
		state:   make(map[string]GossipState),
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

func main() {
	var address, peers, label string
	var port int

	flag.StringVar(&address, "address", "localhost", "Address to listen on")
	flag.IntVar(&port, "port", 0, "Port to listen on")
	flag.StringVar(&peers, "peers", "", "Address of peers")
	flag.StringVar(&label, "label", "", "label for this cluster")
	flag.Parse()

	config := memberlist.DefaultLocalConfig()
	config.BindAddr = address
	config.BindPort = port
	config.Name = strconv.Itoa(port)
	config.Label = label
	config.EnableCompression = true
	config.DeadNodeReclaimTime = 5 * time.Minute
	config.ProtocolVersion = memberlist.ProtocolVersionMax
	config.DelegateProtocolVersion = memberlist.ProtocolVersionMax
	config.DelegateProtocolMin = memberlist.ProtocolVersion2Compatible
	config.DelegateProtocolMax = memberlist.ProtocolVersionMax
	config.LogOutput = io.Discard

	delegate, err := NewBreakerDelegate(config)
	if err != nil {
		log.Fatalln("failed to create memberlist", err)
	}

	delegate.Join(strings.Fields(peers))

	// Create a channel to listen for exit signals
	stop := make(chan os.Signal, 1)
	ticker := time.NewTicker(10 * time.Second)

	// Register the signals we want to be notified, these 3 indicate exit
	// signals, similar to CTRL+C
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Continue doing whatever you need, memberlist will maintain membership
	// information in the background. Delegates can be used for receiving
	// events when members join or leave.
	for {
		select {
		case <-stop:
			if err := delegate.cluster.Leave(15 * time.Second); err != nil {
				log.Fatalln("failed to gracefully leave the cluster", err)
			}

			if err := delegate.cluster.Shutdown(); err != nil {
				log.Fatalln("failed to shutdown gossip listeners", err)
			}

			return
		case <-ticker.C:
			fmt.Println("Alive members:")
			for _, member := range delegate.cluster.Members() {
				if member == delegate.cluster.LocalNode() {
					continue
				}

				fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
			}
		}
	}
}
