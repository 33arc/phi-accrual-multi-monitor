package main

import (
	// "flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/monitor"
	"github.com/33arc/phi-accrual-multi-monitor/node"
	"github.com/33arc/phi-accrual-multi-monitor/server"

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

	// parse cli flags
	p := flags.NewParser(&opts, flags.Default)
	if _, err := p.ParseArgs(os.Args[1:]); err != nil {
		log.Panicln(err)
	}

	// if we are the leader...
	var cfg config.Config
	if opts.ConfigFile != "" && opts.JoinAddress == "" {
		cfg, err = config.Load(opts.ConfigFile)
		if err != nil {
			log.Printf("[WARN] Error loading YAML config: %v", err)
		}
	}

	log.Printf("[INFO] '%s' is used to store files of the node", opts.DataDir)

	config := node.Config{
		BindAddress:    opts.BindAddress,
		NodeIdentifier: opts.BindAddress,
		JoinAddress:    opts.JoinAddress,
		DataDir:        opts.DataDir,
		Bootstrap:      opts.Bootstrap,
	}
	storage, err := node.NewRStorage(&config)
	if err != nil {
		log.Panic(err)
	}

	msg := fmt.Sprintf("[INFO] Started node=%s", storage.RaftNode)
	log.Println(msg)

	go printStatus(storage)

	// If JoinAddress is not nil and there is no cluster, we have to send a POST request to this address
	// It must be an address of the cluster leader
	// We send POST request every second until it succeed
	if config.JoinAddress != "" {
		for 1 == 1 {
			time.Sleep(time.Second * 1)
			err := storage.JoinCluster(config.JoinAddress)
			if err != nil {
				log.Printf("[ERROR] Can't join the cluster: %+v", err)
			} else {
				break
			}
		}
	}

	var wg sync.WaitGroup
	backupChan := make(chan int) // Channel to signal backup received

	for _, server := range cfg.Servers {
		sm, err := monitor.NewServerMonitor(server)
		if err != nil {
			fmt.Printf("Error creating monitor for server %d: %v\n", server.ID, err)
			continue
		}

		wg.Add(1)

		// go sm.MonitorServer(&wg)
		go func(s *monitor.ServerMonitor) {
			defer wg.Done()
			if opts.JoinAddress != "" {
				// Only wait for backup if join address is specified
				if err := s.WaitForBackup(backupChan); err != nil {
					fmt.Printf("Error waiting for backup for server %d: %v\n", server.ID, err)
					return
				}
			}
			s.MonitorServer()
		}(sm)

		// Start a goroutine to check for backup only if join address is specified
		if opts.JoinAddress != "" {
			go func(serverID int) {
				if err := checkBackup(serverID); err != nil {
					fmt.Printf("Error checking backup for server %d: %v\n", serverID, err)
					return
				}
				backupChan <- serverID
			}(server.ID)
		}
	}

	go server.RunHTTPServer(storage)

	wg.Wait()
}

func checkBackup(serverID int) error {
	// implement the logic to check if the backup is received
	return nil
}

func printStatus(s *node.RStorage) {
	for 1 == 1 {
		log.Printf("[DEBUG] state=%s leader=%s", s.RaftNode.State(), s.RaftNode.Leader())
		time.Sleep(time.Second * 2)
	}
}
