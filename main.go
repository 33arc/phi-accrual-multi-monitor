package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/monitor"
	"github.com/33arc/phi-accrual-multi-monitor/node"

	"github.com/jessevdk/go-flags"
)

// Opts represents command line options
type Opts struct {
	BindAddress string `long:"bind" env:"BIND" default:"127.0.0.1:3000" description:"ip:port to bind for a node"`
	JoinAddress string `long:"join" env:"JOIN" default:"" description:"ip:port to join for a node"`
	Bootstrap   bool   `long:"bootstrap" env:"BOOTSTRAP" description:"bootstrap a cluster"`
	DataDir     string `long:"datadir" env:"DATA_DIR" default:"/tmp/data/" description:"Where to store system data"`
	ConfigFile  string `yaml:"-" long:"config" default:"servers.yml" description:"Path to YAML config file"`
}

func main() {
	var opts Opts
	var err error

	p := flags.NewParser(&opts, flags.Default)
	if _, err := p.ParseArgs(os.Args[1:]); err != nil {
		log.Panicln(err)
	}

	log.Printf("[INFO] '%s' is used to store files of the node", opts.DataDir)

	nodeConfig := node.Config{
		BindAddress:    opts.BindAddress,
		NodeIdentifier: opts.BindAddress,
		JoinAddress:    opts.JoinAddress,
		DataDir:        opts.DataDir,
		Bootstrap:      opts.Bootstrap,
	}
	storage, err := node.NewRStorage(&nodeConfig)
	if err != nil {
		log.Panic(err)
	}

	msg := fmt.Sprintf("[INFO] Started node=%s", storage.RaftNode)
	log.Println(msg)

	go printStatus(storage)

	// if we are the leader...
	var cfg config.Config
	if opts.ConfigFile != "" && opts.JoinAddress == "" {
		cfg, err = config.Load(opts.ConfigFile)
		if err != nil {
			log.Panicln(err)
		}
		// TODO: set the keys to the storage.RaftNode
	}

	// If JoinAddress is not nil and there is no cluster, we have to send a POST request to this address
	// It must be an address of the cluster leader
	// We send POST request every second until it succeed
	if nodeConfig.JoinAddress != "" {
		for 1 == 1 {
			time.Sleep(time.Second * 1)
			err := storage.JoinCluster(nodeConfig.JoinAddress)
			if err != nil {
				log.Printf("[ERROR] Can't join the cluster: %+v", err)
			} else {
				break
			}
		}
	}

	done := make(chan struct{})
	errChan := make(chan error, len(cfg.Servers))

	for _, server := range cfg.Servers {
		sm, err := monitor.NewServerMonitor(server)
		if err != nil {
			fmt.Printf("Error creating monitor for server %d: %v\n", server.ID, err)
			continue
		}
		go func() {
			errChan <- sm.MonitorServer(done)
		}()
	}

	// wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// wait for either an error or an interrupt
	select {
	case err := <-errChan:
		fmt.Printf("Error in server monitoring: %v\n", err)
	case <-sigChan:
		fmt.Println("Received interrupt signal. Shutting down...")
	}

	close(done) // signal all goroutines to stop

	// wait for all server monitoring goroutines to finish
	for range cfg.Servers {
		<-errChan
	}

	fmt.Println("All server monitoring stopped. Exiting.")
}

func printStatus(s *node.RStorage) {
	for 1 == 1 {
		log.Printf("[DEBUG] state=%s leader=%s", s.RaftNode.State(), s.RaftNode.Leader())
		time.Sleep(time.Second * 2)
	}
}
