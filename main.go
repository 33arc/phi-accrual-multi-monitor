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
	opts := parseCommandLineFlags()
	storage := initializeStorage(opts)
	cfg := setupConfiguration(storage, opts)

	go printStatus(storage)

	monitorServers(cfg, storage)

	waitForShutdownSignal()
}

func parseCommandLineFlags() Opts {
	var opts Opts
	p := flags.NewParser(&opts, flags.Default)
	if _, err := p.ParseArgs(os.Args[1:]); err != nil {
		log.Fatalf("[ERROR] Failed to parse arguments: %v", err)
	}
	return opts
}

func initializeStorage(opts Opts) *node.RStorage {
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

	return storage
}

func setupConfiguration(storage *node.RStorage, opts Opts) *config.Config {
	if opts.Bootstrap {
		return bootstrapConfiguration(storage, opts)
	} else if opts.JoinAddress != "" {
		return joinClusterAndFetchConfig(storage, opts)
	}
	return nil
}

func bootstrapConfiguration(storage *node.RStorage, opts Opts) *config.Config {
	cfg, err := config.Load(opts.ConfigFile)
	if err != nil {
		log.Fatalf("[ERROR] Failed to load config: %v", err)
	}

	waitForLeadership(storage)

	if err := setInitialConfig(storage, cfg); err != nil {
		log.Fatalf("[ERROR] Failed to set initial config: %v", err)
	}

	return cfg
}

func waitForLeadership(storage *node.RStorage) {
	log.Println("[INFO] Waiting to become leader...")
	for storage.RaftNode.State() != raft.Leader {
		time.Sleep(time.Second)
	}
	log.Println("[INFO] Node is now the leader")
}

func setInitialConfig(storage *node.RStorage, cfg *config.Config) error {
	encodedConfig, err := cfg.Encode()
	if err != nil {
		return err
	}

	if err := storage.Set("config", string(encodedConfig)); err != nil {
		return err
	}

	log.Println("[INFO] Initial config set successfully")
	return nil
}

func joinClusterAndFetchConfig(storage *node.RStorage, opts Opts) *config.Config {
	joinCluster(storage, opts.JoinAddress)
	return fetchConfigFromLeader(storage)
}

func joinCluster(storage *node.RStorage, joinAddress string) {
	log.Println("[INFO] Attempting to join cluster...")
	retryOperation(func() error {
		return storage.JoinCluster(joinAddress)
	}, 10, 5*time.Second, "[INFO] Successfully joined the cluster")
}

func fetchConfigFromLeader(storage *node.RStorage) *config.Config {
	log.Println("[INFO] Fetching config from leader...")
	retryOperation(func() error {
		return storage.RequestConfigFromLeader()
	}, 5, 2*time.Second, "[INFO] Successfully fetched config from leader")

	cfgStr, err := storage.Get("config")
	if err != nil {
		log.Fatalf("[ERROR] Failed to get config from storage: %v", err)
	}

	cfg, err := config.DecodeConfig([]byte(cfgStr))
	if err != nil {
		log.Fatalf("[ERROR] Failed to decode config: %v", err)
	}

	return cfg
}

func retryOperation(operation func() error, maxRetries int, delay time.Duration, successMessage string) {
	for i := 0; i < maxRetries; i++ {
		log.Printf("[INFO] Attempt %d/%d", i+1, maxRetries)
		err := operation()
		if err == nil {
			log.Println(successMessage)
			return
		}
		if i == maxRetries-1 {
			log.Fatalf("[ERROR] Operation failed after %d attempts: %v", maxRetries, err)
		}
		log.Printf("[WARN] Attempt failed, retrying: %v", err)
		time.Sleep(delay)
	}
}

func monitorServers(cfg *config.Config, storage *node.RStorage) {
	done := make(chan struct{})
	errChan := make(chan error, len(cfg.Servers))

	for _, server := range cfg.Servers {
		go monitorServer(server, done, errChan)
	}

	go server.RunHTTPServer(storage)
}

func monitorServer(server config.ServerConfig, done chan struct{}, errChan chan error) {
	sm, err := monitor.NewServerMonitor(server)
	if err != nil {
		log.Printf("[ERROR] Error creating monitor for server %d: %v", server.ID, err)
		return
	}
	errChan <- sm.MonitorServer(done)
}

func waitForShutdownSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("[INFO] Received interrupt signal. Shutting down...")
}

func printStatus(s *node.RStorage) {
	for {
		log.Printf("[DEBUG] Raft state: %s, Leader: %s", s.RaftNode.State(), s.RaftNode.Leader())
		time.Sleep(time.Second * 5)
	}
}
