package api

import (
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/liangzd/hapi-lite/internal/session"
)

type MachineHandler struct {
	*BaseHandler
}

func localMachineID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "local"
	}
	return host
}

func (h *MachineHandler) List(c *gin.Context) {
	host, _ := os.Hostname()
	c.JSON(http.StatusOK, gin.H{
		"machines": []gin.H{
			{
				"id":     localMachineID(),
				"active": true,
				"metadata": gin.H{
					"host":            host,
					"platform":        runtime.GOOS,
					"happyCliVersion": "hapi-lite",
					"displayName":     host,
				},
			},
		},
	})
}

func (h *MachineHandler) Spawn(c *gin.Context) {
	machineID := c.Param("id")
	if machineID != localMachineID() && machineID != "local" {
		c.JSON(http.StatusNotFound, gin.H{"type": "error", "message": "Machine not found"})
		return
	}

	var req session.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "message": "Invalid body"})
		return
	}
	if strings.TrimSpace(req.Directory) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "message": "directory is required"})
		return
	}
	if req.Agent == "" {
		req.Agent = "claude"
	}

	sess, err := h.Store.CreateSession(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "message": err.Error()})
		return
	}
	if h.Mgr != nil {
		startSeq, countErr := h.Store.GetMessageCount(sess.ID)
		if countErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "message": countErr.Error()})
			return
		}
		h.Mgr.SpawnAgent(sess.ID, req, startSeq)
	}
	if h.Broker != nil {
		h.Broker.Publish(session.SyncEvent{
			Type: "session-added", SessionID: sess.ID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"type": "success", "sessionId": sess.ID})
}

func (h *MachineHandler) PathsExists(c *gin.Context) {
	machineID := c.Param("id")
	if machineID != localMachineID() && machineID != "local" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Machine not found"})
		return
	}

	var body struct {
		Paths []string `json:"paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}

	exists := make(map[string]bool, len(body.Paths))
	for _, p := range body.Paths {
		path := strings.TrimSpace(p)
		if path == "" {
			continue
		}
		_, err := os.Stat(path)
		exists[p] = err == nil
	}

	c.JSON(http.StatusOK, gin.H{"exists": exists})
}
