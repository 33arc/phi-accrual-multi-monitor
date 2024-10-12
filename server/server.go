package server

import (
	// "encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/33arc/phi-accrual-multi-monitor/config"
	"github.com/33arc/phi-accrual-multi-monitor/node"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type joinData struct {
	Address string `json:"address"`
}

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
	return func(c *gin.Context) {
		key := c.Param("key")
		val, err := storage.Get(key)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", val)
	}
}

func setKeyView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
		key := c.Param("key")
		value, err := c.GetRawData()
		if err != nil {
			log.Printf("[ERROR] Reading POST data error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		err = storage.Set(key, value)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": fmt.Sprintf("%v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Value set successfully"})
	}
}

func setupRouter(raftNode *node.RStorage) *gin.Engine {
	router := gin.Default()

	router.POST("/cluster/join/", joinView(raftNode))
	router.GET("/keys/:key/", getKeyView(raftNode))
	router.POST("/keys/:key/", setKeyView(raftNode))
	router.GET("/metrics", wrapPromHandler())

	router.GET("/config", getConfigView(raftNode))
	router.POST("/config", setConfigView(raftNode))
	router.PATCH("/config", patchConfigView(raftNode))

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
			log.Printf("[ERROR] Failed to get config: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Config not found or not initialized yet"})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", config)
	}
}

func setConfigView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
		configData, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		if err := storage.Set("config", configData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set config"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "Config updated"})
	}
}

func patchConfigView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
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

		if err := storage.Patch("config", serverConfig); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to patch config"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "Config patched successfully"})
	}
}
