package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/phidetector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
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

type ServerMonitor struct {
	config   ServerConfig
	detector *phidetector.PhiAccrualFailureDetector
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

func (sm *ServerMonitor) monitorServer(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		start := time.Now()
		err := pingServer(sm.config.URL)
		end := time.Now()
		duration := end.Sub(start)
		timestampMillis := end.UnixNano() / int64(time.Millisecond)
		if err == nil {
			sm.detector.Heartbeat(timestampMillis)
		}
		phi := sm.detector.Phi(timestampMillis)
		phiGauge.WithLabelValues(fmt.Sprintf("server%d", sm.config.ID)).Set(phi)
		fmt.Printf("Server %d: Phi = %f, Latency = %v, Error = %v, Timestamp = %v\n",
			sm.config.ID, phi, duration, err, end.Format(time.RFC3339Nano))
		time.Sleep(1 * time.Second)
	}
}

func createDetector() (*phidetector.PhiAccrualFailureDetector, error) {
	return phidetector.NewBuilder().
		SetThreshold(16.0).
		SetMaxSampleSize(200).
		SetMinStdDeviationMillis(500).
		SetAcceptableHeartbeatPauseMillis(0).
		SetFirstHeartbeatEstimateMillis(500).
		Build()
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

	var wg sync.WaitGroup

	// Create and start a ServerMonitor for each server
	for _, server := range config.Servers {
		detector, err := createDetector()
		if err != nil {
			fmt.Printf("Error creating detector for server %d: %v\n", server.ID, err)
			continue
		}

		monitor := &ServerMonitor{
			config:   server,
			detector: detector,
		}

		wg.Add(1)
		go monitor.monitorServer(&wg)
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
