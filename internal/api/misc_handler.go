package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type MiscHandler struct{}

func (h *MiscHandler) Visibility(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *MiscHandler) PushVapidPublicKey(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Push notifications are not enabled in hapi-lite"})
}

func (h *MiscHandler) PushSubscribe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *MiscHandler) PushUnsubscribe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
