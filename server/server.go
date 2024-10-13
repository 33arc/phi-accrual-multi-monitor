package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/node"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type joinData struct {
	Address string `json:"address"`
}

// joinView handles 'join' request from another nodes
// if some node wants to join to the cluster it must be added by leader
// so this node sends a POST request to the leader with it's address and the leades adds it as a voter
// if this node os not a leader, it forwards request to current cluster leader
func joinView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
		var data joinData
		if err := c.BindJSON(&data); err != nil {
			log.Printf("[ERROR] Reading POST data error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		log.Printf("[INFO] Received join request from: %s", data.Address)

		if err := storage.AddVoter(data.Address); err != nil {
			log.Printf("[ERROR] Failed to add voter: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add voter"})
			return
		}

		log.Printf("[INFO] Successfully added voter: %s", data.Address)
		c.JSON(http.StatusOK, gin.H{"message": "Successfully joined the cluster"})
	}
}

func getKeyView(storage *node.RStorage) func(*gin.Context) {
	view := func(c *gin.Context) {
		key := c.Param("key")
		val, err := storage.Get(key)
		if err != nil {
			c.JSON(500, "err")
		}
		c.JSON(200, gin.H{
			"value": val,
		})
	}
	return view
}

type setKeyData struct {
	Value []byte `json:"value"`
}

func setKeyView(storage *node.RStorage) func(*gin.Context) {
	view := func(c *gin.Context) {
		if storage.RaftNode.State() != raft.Leader {
			// http post
			// return MOVED to user
		}
		key := c.Param("key")
		var data setKeyData
		err := c.BindJSON(&data)
		if err != nil {
			log.Printf("[ERROR] Reading POST data error: %+v", err)
			c.JSON(503, gin.H{})
		}

		err = storage.Set(key, data.Value)
		if err != nil {
			c.JSON(503, gin.H{
				"code":  "some_code", // todo :)
				"error": fmt.Sprintf("%+v", err),
			})
		} else {
			c.JSON(200, gin.H{
				"value": data.Value,
			})
		}
	}
	return view
}

func setupRouter(raftNode *node.RStorage) *gin.Engine {
	router := gin.Default()

	router.POST("/cluster/join/", joinView(raftNode))
	router.GET("/keys/:key/", getKeyView(raftNode))
	router.POST("/keys/:key/", setKeyView(raftNode))
	router.GET("/metrics", wrapPromHandler())

	router.GET("/config", getConfigView(raftNode))
	router.POST("/config", setConfigView(raftNode))
	router.PUT("/config", putConfigView(raftNode))

	return router
}

func wrapPromHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func RunHTTPServer(raftNode *node.RStorage) {
	router := setupRouter(raftNode)
	router.Run(raftNode.Config.HTTPBindAddress)
}

func getConfigView(storage *node.RStorage) gin.HandlerFunc {
	return func(c *gin.Context) {
		config, err := storage.Get("config")
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Config not found or not initialized yet"})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", []byte(config))
	}
}
func setConfigView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
		if storage.RaftNode.State() != raft.Leader {
			forwardToLeader(c, storage, "/config", http.MethodPost)
			return
		}
		var data struct {
			Config []byte `json:"config"`
		}
		if err := c.BindJSON(&data); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}
		if err := storage.Set("config", data.Config); err != nil {
			c.JSON(500, gin.H{"error": "Failed to set config"})
			return
		}
		c.JSON(200, gin.H{"status": "Config updated"})
	}
}

func putConfigView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
		if storage.RaftNode.State() != raft.Leader {
			forwardToLeader(c, storage, "/config", http.MethodPut)
			return
		}
		var serverConfig config.ServerConfig
		if err := c.BindJSON(&serverConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		log.Printf("Server %d:\n", serverConfig.ID)
		log.Printf("  Threshold: %f\n", serverConfig.Monitor.Threshold)
		log.Printf("  MaxSampleSize: %d\n", serverConfig.Monitor.MaxSampleSize)
		log.Printf("  MinStdDeviationMillis: %f\n", serverConfig.Monitor.MinStdDeviationMillis)
		log.Printf("  AcceptableHeartbeatPauseMillis: %d\n", serverConfig.Monitor.AcceptableHeartbeatPauseMillis)
		log.Printf("  FirstHeartbeatEstimateMillis: %d\n", serverConfig.Monitor.FirstHeartbeatEstimateMillis)

		if err := storage.Put("config", serverConfig); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to patch config"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "Config patched successfully"})
	}
}

func forwardToLeader(c *gin.Context, storage *node.RStorage, path string, method string) {
	leaderAddr := storage.RaftNode.Leader()
	if leaderAddr == "" {
		log.Println("Error: No leader available")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No leader available"})
		return
	}

	// Parse the leader's Raft address
	host, portStr, err := net.SplitHostPort(string(leaderAddr))
	if err != nil {
		log.Printf("Error parsing leader address: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse leader address"})
		return
	}

	// Convert Raft port to int
	raftPort, err := strconv.Atoi(portStr)
	if err != nil {
		log.Printf("Error converting port to integer: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse leader port"})
		return
	}

	// Calculate HTTP port
	httpPort := raftPort + 1000

	leaderURL := fmt.Sprintf("http://%s:%d%s", host, httpPort, path)
	log.Printf("Forwarding request to leader: %s %s", method, leaderURL)

	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
		return
	}

	// Create a new request to the leader
	req, err := http.NewRequest(method, leaderURL, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error creating request to leader: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request to leader"})
		return
	}

	// Copy headers from the original request
	for name, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Send the request to the leader
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error forwarding request to leader: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Failed to forward request to leader: %v", err)})
		return
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading leader response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read leader response"})
		return
	}

	// Set the response status code
	c.Status(resp.StatusCode)

	// Set the response headers
	for name, values := range resp.Header {
		for _, value := range values {
			c.Header(name, value)
		}
	}

	// Write the response body
	c.Writer.Write(respBody)
	log.Printf("Successfully forwarded request to leader and received response with status: %d", resp.StatusCode)
}
