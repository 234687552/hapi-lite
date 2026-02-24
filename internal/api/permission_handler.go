package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PermissionHandler struct {
	*BaseHandler
}

func (h *PermissionHandler) Approve(c *gin.Context) {
	sessionID := c.Param("id")
	requestID := c.Param("requestId")

	var body struct {
		Mode       string      `json:"mode,omitempty"`
		AllowTools []string    `json:"allowTools,omitempty"`
		Decision   string      `json:"decision,omitempty"`
		Answers    interface{} `json:"answers,omitempty"`
	}
	c.ShouldBindJSON(&body)

	if h.Mgr != nil {
		h.Mgr.ApprovePermission(sessionID, requestID, body.Decision, body.Answers)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *PermissionHandler) Deny(c *gin.Context) {
	sessionID := c.Param("id")
	requestID := c.Param("requestId")

	if h.Mgr != nil {
		h.Mgr.DenyPermission(sessionID, requestID)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
