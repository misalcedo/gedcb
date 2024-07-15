package main

import (
	"context"
	"flag"
	"github.com/hashicorp/memberlist"
	"log"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var address, cluster string
	var port int

	flag.StringVar(&address, "address", "localhost", "address of the current node")
	flag.StringVar(&cluster, "cluster", "localhost", "address of the cluster")
	flag.IntVar(&port, "port", 0, "port of the node")
	flag.Parse()

	config := memberlist.DefaultLocalConfig()
	config.Label = cluster
	config.BindPort = port
	config.AdvertisePort = port
	config.EnableCompression = true
	config.DeadNodeReclaimTime = 5 * time.Minute
	config.ProtocolVersion = memberlist.ProtocolVersionMax
	config.DelegateProtocolVersion = memberlist.ProtocolVersionMax
	config.DelegateProtocolMin = memberlist.ProtocolVersion2Compatible
	config.DelegateProtocolMax = memberlist.ProtocolVersionMax

	delegate, err := NewBreakerDelegate(config)
	if err != nil {
		log.Fatalln("failed to create memberlist", err)
	}

	err = delegate.FastJoin(cluster)
	if err != nil {
		log.Println("failed to join the cluster quickly", err)
	}

	// Join the rest of the cluster in the background
	joinCtx, stopJoin := context.WithTimeout(ctx, 10*time.Minute)
	defer stopJoin()
	go func() {
		err = delegate.Join(joinCtx, cluster)
		if err != nil {
			log.Println("failed to join cluster", err)
		}
	}()

	ticker := time.NewTicker(10 * time.Second)

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
