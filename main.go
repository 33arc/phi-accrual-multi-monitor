package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/monitor"
	"github.com/33arc/phi-accrual-multi-monitor/node"
	"github.com/33arc/phi-accrual-multi-monitor/server"
	"github.com/hashicorp/raft"

	"github.com/jessevdk/go-flags"
)

type Opts struct {
	RaftBindAddress string `long:"raft-bind" env:"RAFT_BIND" default:"127.0.0.1:7000" description:"ip:port to bind for Raft communication"`
	HTTPBindAddress string `long:"http-bind" env:"HTTP_BIND" default:"127.0.0.1:8000" description:"ip:port to bind for HTTP API"`
	JoinAddress     string `long:"join" env:"JOIN" default:"" description:"HTTP address of a node to join the cluster"`
	Bootstrap       bool   `long:"bootstrap" env:"BOOTSTRAP" description:"bootstrap a cluster"`
	DataDir         string `long:"datadir" env:"DATA_DIR" default:"/tmp/data/" description:"Where to store system data"`
	ConfigFile      string `yaml:"-" long:"config" default:"" description:"Path to YAML config file"`
}

func main() {
	var opts Opts
	var err error

	p := flags.NewParser(&opts, flags.Default)
	if _, err := p.ParseArgs(os.Args[1:]); err != nil {
		log.Fatalf("[ERROR] Failed to parse arguments: %v", err)
	}

	log.Printf("[INFO] Using data directory: %s", opts.DataDir)

	nodeConfig := node.StorageConfig{
		RaftBindAddress: opts.RaftBindAddress,
		HTTPBindAddress: opts.HTTPBindAddress,
		NodeIdentifier:  opts.RaftBindAddress,
		JoinAddress:     opts.JoinAddress,
		DataDir:         opts.DataDir,
		Bootstrap:       opts.Bootstrap,
	}

	storage, err := node.NewRStorage(&nodeConfig)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create storage: %v", err)
	}

	if storage == nil {
		log.Fatalf("[ERROR] Storage is nil after creation")
	}

	var cfg *config.Config

	if opts.Bootstrap {
		cfg, err = config.Load(opts.ConfigFile)
		if err != nil {
			log.Fatalf("[ERROR] Failed to load config: %v", err)
		}

		log.Println("[INFO] Waiting to become leader...")
		for storage.RaftNode.State() != raft.Leader {
			time.Sleep(time.Second)
		}
		log.Println("[INFO] Node is now the leader")

		encodedConfig, err := cfg.Encode()
		if err != nil {
			log.Fatalf("[ERROR] Failed to encode config: %v", err)
		}

		// Use log.Printf to format the output correctly
		// log.Printf("STRING ENCODED CONFIG SIZE: %d", len(encodedConfig))

		if err := storage.Set("config", string(encodedConfig)); err != nil {
			log.Fatalf("[ERROR] Failed to set initial config: %v", err)
		}

		// expected, err := storage.Get("config")
		// if err != nil {
		// 	log.Panicln(err)
		// }

		// No need to convert expected to bytes; just compare directly
		// if string(encodedConfig) == expected {
		// 	log.Println("encoded and stored values match")
		// } else {
		// 	log.Fatalf("encoded and stored values DO NOT MATCH")
		// }
		log.Println("[INFO] Initial config set successfully")
	} else if opts.JoinAddress != "" {
		log.Println("[INFO] Attempting to join cluster...")

		maxRetries := 10
		for i := 0; i < maxRetries; i++ {
			log.Printf("[INFO] Joining cluster attempt %d/%d", i+1, maxRetries)
			err := storage.JoinCluster(opts.JoinAddress)
			if err == nil {
				log.Println("[INFO] Successfully joined the cluster")
				break
			}
			if i == maxRetries-1 {
				log.Fatalf("[ERROR] Failed to join cluster after %d attempts: %v", maxRetries, err)
			}
			log.Printf("[WARN] Failed to join cluster, retrying: %v", err)
			time.Sleep(time.Second * 5)
		}

		log.Println("[INFO] Fetching config from leader...")
		maxConfigRetries := 5
		for i := 0; i < maxConfigRetries; i++ {
			err := storage.RequestConfigFromLeader()
			if err == nil {
				log.Println("[INFO] Successfully fetched config from leader")
				break
			}
			if i == maxConfigRetries-1 {
				log.Fatalf("[ERROR] Failed to fetch config after %d attempts: %v", maxConfigRetries, err)
			}
			log.Printf("[WARN] Failed to fetch config, retrying: %v", err)
			time.Sleep(time.Second * 2)
		}

		cfgStr, err := storage.Get("config")
		if err != nil {
			log.Fatalf("[ERROR] Failed to get config from storage: %v", err)
		}
		// log.Println("STRING ENCODED CONFIG SIZE %d", len(cfgStr))
		cfg, err = config.DecodeConfig([]byte(cfgStr))
		if err != nil {
			log.Fatalf("[ERROR] Failed to decode config: %v", err)
		}
	}

	log.Printf("[INFO] Started node: %s", storage.RaftNode.String())

	go printStatus(storage)

	done := make(chan struct{})
	errChan := make(chan error, len(cfg.Servers))

	for _, server := range cfg.Servers {
		sm, err := monitor.NewServerMonitor(server)
		if err != nil {
			log.Printf("[ERROR] Error creating monitor for server %d: %v", server.ID, err)
			continue
		}
		go func() {
			errChan <- sm.MonitorServer(done)
		}()
	}

	go server.RunHTTPServer(storage)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		log.Printf("[ERROR] Error in server monitoring: %v", err)
	case <-sigChan:
		log.Println("[INFO] Received interrupt signal. Shutting down...")
	}

	close(done)

	for range cfg.Servers {
		<-errChan
	}

	log.Println("[INFO] All server monitoring stopped. Exiting.")
}

func printStatus(s *node.RStorage) {
	for {
		log.Printf("[DEBUG] Raft state: %s, Leader: %s", s.RaftNode.State(), s.RaftNode.Leader())
		time.Sleep(time.Second * 5)
	}
}
