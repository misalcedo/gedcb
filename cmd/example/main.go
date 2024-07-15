package main

import (
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
