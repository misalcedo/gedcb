// package main contains a simple gossip cluster.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/memberlist"
)

type seedNodes []string

// String is an implementation of the flag.Value interface
func (i *seedNodes) String() string {
	return fmt.Sprintf("%v", *i)
}

// Set is an implementation of the flag.Value interface
func (i *seedNodes) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type clusterDelegate struct {
}

func (c *clusterDelegate) NodeMeta(limit int) []byte {
	//TODO implement me
	panic("implement me")
}

func (c *clusterDelegate) NotifyMsg(bytes []byte) {
	//TODO implement me
	panic("implement me")
}

func (c *clusterDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	//TODO implement me
	panic("implement me")
}

func (c *clusterDelegate) LocalState(join bool) []byte {
	//TODO implement me
	panic("implement me")
}

func (c *clusterDelegate) MergeRemoteState(buf []byte, join bool) {
	//TODO implement me
	panic("implement me")
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var seeds seedNodes
	var cluster, peers string
	var gossipPort, httpPort int

	flag.Var(&seeds, "seed", "the hostname of a seed node")
	flag.StringVar(&cluster, "cluster", "", "address of the cluster")
	flag.StringVar(&peers, "peers", "", "list of peers to join the cluster")
	flag.IntVar(&gossipPort, "gossipPort", 7946, "port for the node to gossip on")
	flag.IntVar(&httpPort, "httpPort", 8080, "port of the node to start the HTTP server on")
	flag.Parse()

	config := memberlist.DefaultWANConfig()

	config.Name = os.Getenv("HOSTNAME")
	config.Label = cluster
	config.BindPort = gossipPort
	config.DeadNodeReclaimTime = 1 * time.Second
	config.ProtocolVersion = memberlist.ProtocolVersionMax
	config.DelegateProtocolVersion = memberlist.ProtocolVersionMax
	config.DelegateProtocolMin = memberlist.ProtocolVersion2Compatible
	config.DelegateProtocolMax = memberlist.ProtocolVersionMax
	config.LogOutput = io.Discard
	config.Delegate = &clusterDelegate{}

	log.SetPrefix(fmt.Sprintf("[%s] ", config.Name))
	log.Printf("starting node %s with seeds %v\n", config.Name, seeds)

	list, err := memberlist.Create(config)
	if err != nil {
		log.Fatalf("Failed to create memberlist: %v\n", err)
	}

	// Don't join a cluster of just the current pod.
	slices.DeleteFunc(seeds, func(s string) bool {
		return strings.HasPrefix(s, config.Name)
	})

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(seeds)
	if err != nil {
		log.Printf("Failed to join cluster: %v\n", err)
	}

	server := buildHTTPServer(httpPort)
	go listenAndServe(server)

	for {
		select {
		case <-ctx.Done():
			if err = server.Shutdown(context.Background()); err != nil {
				log.Fatalln("failed to gracefully shutdown the server", err)
			}

			return
		case <-time.Tick(time.Second):
			if list.NumMembers() < len(seeds) {
				log.Printf("Reseeding due to low member count: %d\n", list.NumMembers())
				rand.Shuffle(len(peers), func(i, j int) {
					seeds[i], seeds[j] = seeds[j], seeds[i]
				})
				_, err = list.Join(seeds)
				if err != nil {
					log.Printf("Failed to join cluster: %v\n", err)
				}
			}

			for i, member := range list.Members() {
				log.Printf("Member %d: %s %s\n", i, member.Name, member.Addr)
			}
		}
	}
}

func listenAndServe(server *http.Server) {
	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalln(err)
	}
}

func buildHTTPServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: mux,
	}
	return server
}
