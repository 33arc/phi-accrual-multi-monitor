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

	for _, server := range cfg.Servers {
		sm, err := monitor.NewServerMonitor(server)
		if err != nil {
			fmt.Printf("Error creating monitor for server %d: %v\n", server.ID, err)
			continue
		}
		wg.Add(1)
		go sm.MonitorServer(&wg)
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
