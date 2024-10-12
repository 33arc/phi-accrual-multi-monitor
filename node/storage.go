package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/monitor"
	"github.com/hashicorp/raft"
)

type logEvent struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

// Get retrieves the value associated with the given key from storage.
func (s *RStorage) Get(key string) ([]byte, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	value, ok := s.storage[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	return value, nil
}

// Set stores the given value by key in the storage.
func (s *RStorage) Set(key string, value []byte) error {
	if s.RaftNode.State() != raft.Leader {
		return fmt.Errorf("only leader can write to the storage")
	}

	// Prepare the log event
	event := logEvent{
		Type:  "set",
		Key:   key,
		Value: value,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Apply to Raft
	timeout := time.Second * 5
	future := s.RaftNode.Apply(data, timeout)
	if err := future.Error(); err != nil {
		return err
	}

	return nil
}

func (s *RStorage) Patch(key string, value config.ServerConfig) error {
	if s.RaftNode.State() != raft.Leader {
		return fmt.Errorf("only leader can write to the storage")
	}
	encodedServerConfig, err := value.Encode()
	if err != nil {
		return err
	}

	log.Printf("encoded length is %d", len(encodedServerConfig))
	log.Printf("normal ID is %d", value.ID)
	decodedConfig, err := config.DecodeServerConfig(encodedServerConfig)
	if err != nil {
		log.Printf("Error decoding ServerConfig: %v", err)
	} else {
		log.Printf("after decoding, ID is %d", decodedConfig.ID)
	}

	// Prepare the log event
	event := logEvent{
		Type:  "patch",
		Key:   key,
		Value: encodedServerConfig,
	}
	log.Printf("logevent.Value length is %d", len(event.Value))
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Apply to Raft
	timeout := time.Second * 5
	future := s.RaftNode.Apply(data, timeout)
	if err := future.Error(); err != nil {
		return err
	}

	return nil
}

// Apply applies a Raft log entry to the key-value store.
func (s *RStorage) Apply(logEntry *raft.Log) interface{} {
	s.logger.Debug("Applying a new log entry to the store")
	var event logEvent
	if err := json.Unmarshal(logEntry.Data, &event); err != nil {
		s.logger.Error("Can't read Raft log event", "error", err)
		return nil
	}
	log.Printf("logevent.Value length is %d", len(event.Value))

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if event.Type == "set" {
		s.logger.Debug("Set operation received", "key", event.Key)
		s.storage[event.Key] = event.Value
		return nil
	}

	if event.Type == "patch" {
		return s.applyPatch(event)
	}

	s.logger.Warn("Unknown Raft log event type", "type", event.Type)
	return nil
}

func (s *RStorage) applyPatch(event logEvent) interface{} {
	// Get the current configuration
	configBytes, ok := s.storage["config"]
	if !ok {
		s.logger.Error("Failed to get current config")
		return nil
	}

	decodedCfg, err := config.DecodeConfig(configBytes)
	if err != nil {
		s.logger.Error("Failed to decode config(DecodeConfig)", "error", err)
		return nil
	}

	cfg, err := config.DecodeServerConfig(event.Value)
	if err != nil {
		s.logger.Error("Failed to decode config(DecodeServerConfig)", "error", err)
		return nil
	}

	s.logger.Debug("Successfully decoded ServerConfig", "ID", cfg.ID)

	// Check if the server already exists in the config
	var doesExist bool = false
	for _, server := range decodedCfg.Servers {
		if server.ID == cfg.ID {
			doesExist = true
			break
		}
	}

	if !doesExist {
		log.Printf("[INFO] New server detected. Starting monitoring for server %s", cfg.URL)
		done := make(chan struct{})
		errChan := make(chan error, 1)
		go monitor.MonitorSingleServer(*cfg, done, errChan)

		decodedCfg.Servers = append(decodedCfg.Servers, *cfg)
		encodedNewCfgState, err := decodedCfg.Encode()
		if err != nil {
			s.logger.Error("Can't encode the new Config state", "error", err)
			return nil
		}

		s.storage[event.Key] = event.Value
		s.storage["config"] = encodedNewCfgState
	}
	return nil
}

func (s *RStorage) DistributeConfig() error {
	if s.RaftNode.State() != raft.Leader {
		return nil // Only the leader should distribute config
	}

	config, err := s.Get("config")
	if err != nil {
		return fmt.Errorf("failed to get config: %v", err)
	}
	if len(config) == 0 {
		return errors.New("no config found to distribute")
	}

	servers, err := s.GetClusterServers()
	if err != nil {
		return fmt.Errorf("failed to get cluster servers: %v", err)
	}

	localID := s.Config.NodeIdentifier

	for _, server := range servers {
		if server.ID != raft.ServerID(localID) {
			go func(serverAddr raft.ServerAddress) {
				err := s.sendConfigToFollower(string(serverAddr), config)
				if err != nil {
					s.logger.Error("Failed to send config to follower", "serverAddr", serverAddr, "error", err)
				}
			}(server.Address)
		}
	}
	return nil
}

func (s *RStorage) sendConfigToFollower(serverAddr string, config []byte) error {
	data := map[string][]byte{"config": config}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		fmt.Sprintf("http://%s/config", serverAddr),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send config: status code %d", resp.StatusCode)
	}

	return nil
}

func (s *RStorage) RequestConfigFromLeader() error {
	leaderAddr := s.RaftNode.Leader()
	s.logger.Debug("Requesting config from leader", "leader", leaderAddr)
	if leaderAddr == "" {
		return errors.New("no leader available")
	}

	config, err := s.fetchConfigFromLeader(string(leaderAddr))
	if err != nil {
		return err
	}

	return s.SetLocalConfig(config)
}

func (s *RStorage) fetchConfigFromLeader(leaderAddr string) ([]byte, error) {
	url := fmt.Sprintf("http://%s/config", s.getHTTPAddr(leaderAddr))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch config: status code %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return body, nil
}

// Helper function to convert Raft address to HTTP address
func (s *RStorage) getHTTPAddr(raftAddr string) string {
	host, portStr, _ := net.SplitHostPort(raftAddr)
	port, _ := strconv.Atoi(portStr)
	return fmt.Sprintf("%s:%d", host, port+1000)
}

func (s *RStorage) SetLocalConfig(encodedConfig []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.storage["config"] = encodedConfig
	return nil
}

// fsmSnapshot is used to provide a snapshot of the current
// state of the FSM.
type fsmSnapshot struct {
	storage map[string][]byte
}

// Snapshot returns an FSMSnapshot which can be used to restore the FSM
func (s *RStorage) Snapshot() (raft.FSMSnapshot, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Create a deep copy of the storage map
	storageSnapshot := make(map[string][]byte)
	for k, v := range s.storage {
		copyValue := make([]byte, len(v))
		copy(copyValue, v)
		storageSnapshot[k] = copyValue
	}

	return &fsmSnapshot{storage: storageSnapshot}, nil
}

// Restore is used to restore an FSM from a snapshot
func (s *RStorage) Restore(rc io.ReadCloser) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	decoder := json.NewDecoder(rc)
	var snapshot fsmSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return err
	}

	// Clear the current storage and restore from the snapshot
	s.storage = snapshot.storage
	return nil
}

// Persist should dump all necessary state to the WriteCloser 'sink',
// and call sink.Close() when finished or call sink.Cancel() on error.
func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(f.storage)
	if err != nil {
		sink.Cancel()
		return err
	}

	return sink.Close()
}

// Release is invoked when we are finished with the snapshot.
func (f *fsmSnapshot) Release() {}
