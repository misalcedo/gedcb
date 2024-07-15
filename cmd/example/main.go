package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/hashicorp/memberlist"
	"io"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"
)

func main() {
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

	addresses, err := net.LookupIP(cluster)
	if err != nil {
		log.Fatalln("failed to resolve cluster domain name", err)
	}

	err = delegate.Join(addresses)
	if err != nil {
		log.Fatalln("failed to join cluster", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			if err := delegate.cluster.Leave(30 * time.Second); err != nil {
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
