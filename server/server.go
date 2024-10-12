package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/33arc/phi-accrual-multi-monitor/node"
	"github.com/gin-gonic/gin"
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
			log.Println("ERROR FUCKING ERROR %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Config not found or not initialized yet"})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", []byte(config))
	}
}
func setConfigView(storage *node.RStorage) func(*gin.Context) {
	return func(c *gin.Context) {
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
