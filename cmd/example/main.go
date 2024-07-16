package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hashicorp/memberlist"
	"github.com/misalcedo/gedcb"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var address, cluster, name, peers string
	var gossipPort, httpPort int

	flag.StringVar(&name, "name", "", "name of the current node")
	flag.StringVar(&address, "address", "", "address of the current node")
	flag.StringVar(&cluster, "cluster", "", "address of the cluster")
	flag.StringVar(&peers, "peers", "", "list of peers to join the cluster")
	flag.IntVar(&gossipPort, "port", 0, "port for the node to gossip on")
	flag.IntVar(&httpPort, "port", 0, "port of the node to start the HTTP server on")
	flag.Parse()

	config := memberlist.DefaultLANConfig()

	if name != "" {
		config.Name = name
	}

	if address != "" {
		config.BindAddr = address
	}

	config.Label = cluster
	config.BindPort = gossipPort
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

	go joinCluster(ctx, delegate, cluster, peers)
	go gossip(ctx, delegate)
	launchServer(httpPort, delegate.Breaker())
}

func gossip(ctx context.Context, delegate *ClusterDelegate) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := delegate.cluster.Leave(time.Second); err != nil {
				log.Fatalln("failed to gracefully leave the cluster", err)
			}

			if err := delegate.cluster.Shutdown(); err != nil {
				log.Fatalln("failed to shutdown gossip listeners", err)
			}

			return
		case <-ticker.C:
			log.Println("Alive members:")
			for _, member := range delegate.cluster.Members() {
				if member == delegate.cluster.LocalNode() {
					continue
				}

				log.Printf("- %s\n", member.Name)
			}
		}
	}
}

func joinCluster(ctx context.Context, delegate *ClusterDelegate, cluster string, peers string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	err := delegate.Join(cluster, strings.Fields(peers))
	if err != nil {
		log.Println("failed to join cluster", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err = delegate.Join(cluster, strings.Fields(peers))
			if err != nil {
				log.Println("failed to join cluster", err)
			}

			// once we succeed do check less often
			ticker.Reset(time.Minute)
		}
	}
}

type Response struct {
	State     gedcb.State
	Successes int
	Failures  int
}

func launchServer(port int, breaker *gedcb.Breaker) {
	mux := http.NewServeMux()
	mux.HandleFunc("/success", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()

		if err := breaker.Success(now); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		response, err := json.Marshal(Response{
			State:     breaker.State(now),
			Successes: breaker.Successes(now),
			Failures:  breaker.Failures(now),
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, err = io.Copy(w, bytes.NewReader(response))
		if err != nil {
			log.Println("failed to write response", err)
		}
	})
	mux.HandleFunc("/failure", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()

		if err := breaker.Failure(now); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		response, err := json.Marshal(Response{
			State:     breaker.State(now),
			Successes: breaker.Successes(now),
			Failures:  breaker.Failures(now),
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, err = io.Copy(w, bytes.NewReader(response))
		if err != nil {
			log.Println("failed to write response", err)
		}
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()

		response, err := json.Marshal(Response{
			State:     breaker.State(now),
			Successes: breaker.Successes(now),
			Failures:  breaker.Failures(now),
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, err = io.Copy(w, bytes.NewReader(response))
		if err != nil {
			log.Println("failed to write response", err)
		}
	})
	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: mux,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Fatalln(err)
	}
}
