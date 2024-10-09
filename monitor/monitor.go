package monitor

import (
	"fmt"
	"net/http"
	// "sync"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/metrics"
	"github.com/33arc/phi-accrual-multi-monitor/phidetector"
)

type ServerMonitor struct {
	config   config.ServerConfig
	detector *phidetector.PhiAccrualFailureDetector
}

func NewServerMonitor(cfg config.ServerConfig) (*ServerMonitor, error) {
	detector, err := createDetector()
	if err != nil {
		return nil, err
	}
	return &ServerMonitor{
		config:   cfg,
		detector: detector,
	}, nil
}

func (sm *ServerMonitor) WaitForBackup(backupChan <-chan int) error {
	timeout := time.After(3 * time.Second) // TODO: change this
	for {
		select {
		case id := <-backupChan:
			if id == sm.config.ID {
				return nil // backup received.
			}
		case <-timeout:
			return fmt.Errorf("timeout waiting for backup")
		}
	}
}

func (sm *ServerMonitor) MonitorServer() {
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
		metrics.PhiGauge.WithLabelValues(fmt.Sprintf("server%d", sm.config.ID)).Set(phi)
		fmt.Printf("Server %d: Phi = %f, Latency = %v, Error = %v, Timestamp = %v\n",
			sm.config.ID, phi, duration, err, end.Format(time.RFC3339Nano))
		time.Sleep(1 * time.Second) // TODO: this could be done better
	}
}

func pingServer(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
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
