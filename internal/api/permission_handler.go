package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PermissionHandler struct {
	*BaseHandler
}

func (h *PermissionHandler) Approve(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *PermissionHandler) Deny(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
