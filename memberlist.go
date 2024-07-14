package main

import (
	"flag"
	"fmt"
	"github.com/hashicorp/memberlist"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	var address, peers, metadata string
	var port int

	flag.StringVar(&address, "address", "localhost", "Address to listen on")
	flag.IntVar(&port, "port", 0, "Port to listen on")
	flag.StringVar(&peers, "peers", "", "Address of peers")
	flag.StringVar(&metadata, "metadata", "", "Metadata for this node")
	flag.Parse()

	/* Create the initial memberlist from a safe configuration.
	   Please reference the godoc for other default config types.
	   http://godoc.org/github.com/hashicorp/memberlist#Config
	*/
	config := memberlist.DefaultLocalConfig()
	config.BindAddr = address
	config.BindPort = port
	config.Name = strconv.Itoa(port)

	list, err := memberlist.Create(config)
	if err != nil {
		panic("Failed to create memberlist: " + err.Error())
	}

	node := list.LocalNode()
	// You can provide a byte representation of any metadata here. You can broadcast the
	// config for each node in some serialized format like JSON. By default, this is
	// limited to 512 bytes, so may not be suitable for large amounts of data.
	node.Meta = []byte("some metadata")

	// Join an existing cluster by specifying at least one known member.
	_, err = list.Join(strings.Fields(peers))
	if err != nil {
		panic("Failed to join cluster: " + err.Error())
	}

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
			// Leave the cluster with a 5 second timeout. If leaving takes more than 5
			// seconds we return.
			if err := list.Leave(time.Second * 5); err != nil {
				panic(err)
			}
			return
		case <-ticker.C:
			// Ask for members of the cluster
			for _, member := range list.Members() {
				if member == node {
					continue
				}

				fmt.Printf("Member: %s %s\n", member.Name, member.Addr)

				err := list.SendReliable(node, []byte("Hello, world!"))
				if err != nil {
					return
				}
			}
		}
	}
}
