package main

import (
	"flag"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
	"io/ioutil"
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

type ServerConfig struct {
	ID  int    `yaml:"id"`
	URL string `yaml:"url"`
}

type Config struct {
	Servers []ServerConfig `yaml:"servers"`
}

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

func monitorServer(config ServerConfig, detector *phidetector.Detector, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		start := time.Now()
		err := pingServer(config.URL)
		end := time.Now()
		duration := end.Sub(start)
		if err == nil {
			detector.Ping(end)
		}
		phi := detector.Phi(time.Now())
		phiGauge.WithLabelValues(fmt.Sprintf("server%d", config.ID)).Set(phi)
		fmt.Printf("Server %d: Phi = %f, Latency = %v, Error = %v, Timestamp = %v\n",
			config.ID, phi, duration, err, end.Format(time.RFC3339Nano))
		time.Sleep(1 * time.Second)
	}
}

func loadConfig(filename string) (Config, error) {
	var config Config
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	return config, err
}

func main() {
	configFile := flag.String("f", "servers.yml", "path to the configuration file")
	flag.Parse()

	config, err := loadConfig(*configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	const numServers = 10
	const windowSize = 100 // Increased from 10
	const minSamples = 10
	detectors := make([]*phidetector.Detector, len(config.Servers))
	for i := range detectors {
		detectors[i] = phidetector.New(windowSize, minSamples)
	}
	var wg sync.WaitGroup
	// Start a separate goroutine for each server
	for i, server := range config.Servers {
		wg.Add(1)
		go monitorServer(server, detectors[i], &wg)
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
