package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	// Expose Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Starting server on :8080")
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Printf("Error starting metrics server: %v\n", err)
		}
	}()

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
