package main

import (
	"context"
	"flag"
	"github.com/hashicorp/memberlist"
	"io"
	"log"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var cluster string

	flag.StringVar(&cluster, "cluster", "localhost", "address of the cluster")
	flag.Parse()

	config := memberlist.DefaultLocalConfig()
	config.Label = cluster
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

	// Join the cluster in the background
	joinCtx, stopJoin := context.WithTimeout(ctx, 5*time.Minute)
	defer stopJoin()
	go func() {
		err = delegate.Join(joinCtx, cluster)
		if err != nil {
			log.Println("failed to join cluster", err)
		}
	}()

	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-ctx.Done():
			if err := delegate.cluster.Leave(15 * time.Second); err != nil {
				log.Fatalln("failed to gracefully leave the cluster", err)
			}

			if err := delegate.cluster.Shutdown(); err != nil {
				log.Fatalln("failed to shutdown gossip listeners", err)
			}

			return
		case <-ticker.C:
			members := delegate.cluster.Members()

			if len(members) > 1 {
				log.Println("Alive members:")
				for _, member := range members {
					if member == delegate.cluster.LocalNode() {
						continue
					}

					log.Printf("- %s\n", member.Name)
				}
			}
		}
	}
}
