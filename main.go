package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"sync"
	"testy/phidetector"
	"time"
)

var (
	phiGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_phivalue",
			Help: "Phi value for server failure detection",
		},
		[]string{"server"},
	)
)

func init() {
	prometheus.MustRegister(phiGauge)
}

func pingServer(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func monitorServer(id int, detector *phidetector.Detector, serverURL string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		start := time.Now()
		err := pingServer(serverURL)
		end := time.Now()
		duration := end.Sub(start)
		if err == nil {
			detector.Ping(end)
		}
		phi := detector.Phi(time.Now())
		phiGauge.WithLabelValues(fmt.Sprintf("server%d", id)).Set(phi)
		fmt.Printf("Server %d: Phi = %f, Latency = %v, Error = %v, Timestamp = %v\n",
			id, phi, duration, err, end.Format(time.RFC3339Nano))
		time.Sleep(1 * time.Second)
	}
}
func main() {
	const numServers = 10
	const windowSize = 100 // Increased from 10
	const minSamples = 10
	detectors := make([]*phidetector.Detector, numServers)
	for i := range detectors {
		detectors[i] = phidetector.New(windowSize, minSamples)
	}
	var wg sync.WaitGroup
	// Start a separate goroutine for each server
	for i := 0; i < numServers; i++ {
		wg.Add(1)
		serverURL := fmt.Sprintf("http://localhost:%d/heartbeat", 8081+i)
		go monitorServer(i+1, detectors[i], serverURL, &wg)
	}
	// Expose Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Starting server on :8080")
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Printf("Error starting metrics server: %v\n", err)
		}
	}()
	// Wait for all goroutines to finish (which they never will in this case)
	wg.Wait()
}
