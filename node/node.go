package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	rbolt "github.com/hashicorp/raft-boltdb"
)

// Config struct handles configuration for a node
type StorageConfig struct {
	RaftBindAddress string
	HTTPBindAddress string
	NodeIdentifier  string
	JoinAddress     string
	DataDir         string
	Bootstrap       bool
}

// RStorage represents key-value storage with raft based replication
// Also, it represents finite-state machine which processes Raft log events
type RStorage struct {
	mutex    sync.Mutex
	storage  map[string]string
	RaftNode *raft.Raft
	Config   StorageConfig
	logger   hclog.Logger
}

// NewRStorage initiates a new RStorage node
func NewRStorage(config *StorageConfig) (*RStorage, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	rstorage := &RStorage{
		storage: make(map[string]string),
		Config:  *config,
		logger: hclog.New(&hclog.LoggerOptions{
			Name:   "raft-storage",
			Output: os.Stdout,
			Level:  hclog.LevelFromString("INFO"),
		}),
	}

	if err := os.MkdirAll(config.DataDir, 0700); err != nil {
		return nil, err
	}

	// Create a new hclog.Logger
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "raft",
		Output: os.Stdout,
		Level:  hclog.LevelFromString("INFO"),
	})

	rstorage.logger = logger

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.NodeIdentifier)
	raftConfig.Logger = logger

	transport, err := raftTransport(config.RaftBindAddress, logger)
	if err != nil {
		return nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(config.DataDir, 1, os.Stdout)
	if err != nil {
		return nil, err
	}

	logStore, err := rbolt.NewBoltStore(filepath.Join(config.DataDir, "raft-log.bolt"))
	if err != nil {
		return nil, err
	}

	stableStore, err := rbolt.NewBoltStore(filepath.Join(config.DataDir, "raft-stable.bolt"))
	if err != nil {
		return nil, err
	}

	raftNode, err := raft.NewRaft(
		raftConfig,
		rstorage,
		logStore,
		stableStore,
		snapshotStore,
		transport,
	)
	if err != nil {
		return nil, err
	}

	if config.Bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	}

	rstorage.RaftNode = raftNode
	return rstorage, nil
}

func raftTransport(bindAddr string, logger hclog.Logger) (*raft.NetworkTransport, error) {
	address, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(bindAddr, address, 3, 10*time.Second, os.Stdout)
	if err != nil {
		return nil, err
	}

	return transport, nil
}

// GetClusterServers returns all cluster's servers
func (s *RStorage) GetClusterServers() ([]raft.Server, error) {
	configurationFuture := s.RaftNode.GetConfiguration()
	if err := configurationFuture.Error(); err != nil {
		s.logger.Error("Reading Raft configuration error", "error", err)
		return nil, err
	}
	return configurationFuture.Configuration().Servers, nil
}

// AddVoter joins a new voter to a cluster
// must be called only on a leader
func (s *RStorage) AddVoter(address string) error {
	s.logger.Info("Trying to add new voter to the cluster", "address", address)
	addFuture := s.RaftNode.AddVoter(raft.ServerID(address), raft.ServerAddress(address), 0, 0)
	if err := addFuture.Error(); err != nil {
		s.logger.Error("Can't join to the cluster", "error", err)
		return err
	}
	return nil
}

// JoinCluster sends a POST request to "join" address
// to ask the cluster leader join this node as a voter
func (s *RStorage) JoinCluster(leaderHTTPAddress string) error {
	if s == nil {
		return errors.New("RStorage is nil")
	}
	if s.Config.RaftBindAddress == "" {
		return errors.New("RaftBindAddress is not set")
	}
	body, err := json.Marshal(map[string]string{"address": s.Config.RaftBindAddress})
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		fmt.Sprintf("http://%s/cluster/join/", leaderHTTPAddress),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("failed to send join request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("leader returned non-200 status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
