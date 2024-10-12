package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/raft"
)

type logEvent struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

// Get retrieves the value associated with the given key from storage.
func (s *RStorage) Get(key string) ([]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	value, ok := s.storage[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Return the string value directly
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

	// Store in local map for retrieval
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.storage[key] = value // Store the original value
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
	if event.Type == "set" {
		s.logger.Debug("Set operation received", "key", event.Key, "value", event.Value)
		s.mutex.Lock()
		defer s.mutex.Unlock()
		s.storage[event.Key] = event.Value
		return nil
	}
	s.logger.Warn("Unknown Raft log event type", "type", event.Type)
	return nil
}

// fsmSnapshot is used by Raft library to save a point-in-time snapshot of the FSM
type fsmSnapshot struct {
	storage map[string][]byte
}

// Snapshot returns FSMSnapshot which is used to save snapshot of the FSM
func (s *RStorage) Snapshot() (raft.FSMSnapshot, error) {
	s.logger.Debug("Creating snapshot")
	s.mutex.Lock()
	defer s.mutex.Unlock()
	storageCopy := make(map[string][]byte)
	for k, v := range s.storage {
		storageCopy[k] = v
	}
	return &fsmSnapshot{storage: storageCopy}, nil
}

// Restore stores the key-value store to a previous state.
func (s *RStorage) Restore(serialized io.ReadCloser) error {
	s.logger.Debug("Restoring from snapshot")
	var snapshot fsmSnapshot
	if err := json.NewDecoder(serialized).Decode(&snapshot); err != nil {
		return err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.storage = snapshot.storage
	return nil
}

// Persist should dump all necessary state to the WriteCloser 'sink',
// and call sink.Close() when finished or call sink.Cancel() on error.
func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	snapshotBytes, err := json.Marshal(f)
	if err != nil {
		sink.Cancel()
		return err
	}
	if _, err := sink.Write(snapshotBytes); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

// Release is invoked when the Raft library is finished with the snapshot.
func (f *fsmSnapshot) Release() {}

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
	// Assuming the HTTP port is always Raft port + 1000
	// You might need to adjust this based on your actual port configuration
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
