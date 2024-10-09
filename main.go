package main

import (
	"flag"
	"fmt"
	"net/http"
	"sync"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/monitor"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	configFile := flag.String("f", "servers.yml", "path to the configuration file")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
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
			// wait for backup signal
			if err := s.WaitForBackup(backupChan); err != nil {
				fmt.Printf("Error waiting for backup for server %d: %v\n", server.ID, err)
				return
			}
			s.MonitorServer()
		}(sm)

		// Start a goroutine to check for backup
		go func(serverID int) {
			if err := checkBackup(serverID); err != nil {
				fmt.Printf("Error checking backup for server %d: %v\n", serverID, err)
				return
			}
			backupChan <- serverID
		}(server.ID)
	}

	// Expose Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Starting server on :8080")
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Printf("Error starting metrics server: %v\n", err)
		}
	}()

	wg.Wait()
}

func checkBackup(serverID int) error {
	// implement the logic to check if the backup is received
	return nil
}
