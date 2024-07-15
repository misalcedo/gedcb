package main

import (
	"context"
	"flag"
	"github.com/hashicorp/memberlist"
	"io"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var address, cluster, name, peers string
	var port int

	flag.StringVar(&name, "name", "", "name of the current node")
	flag.StringVar(&address, "address", "", "address of the current node")
	flag.StringVar(&cluster, "cluster", "", "address of the cluster")
	flag.StringVar(&peers, "peers", "", "list of peers to join the cluster")
	flag.IntVar(&port, "port", 0, "port of the node")
	flag.Parse()

	config := memberlist.DefaultLANConfig()

	if name != "" {
		config.Name = name
	}

	if address != "" {
		config.BindAddr = address
	}

	config.Label = cluster
	config.BindPort = port
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

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		err = delegate.Join(cluster, strings.Fields(peers))
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
	}()

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
