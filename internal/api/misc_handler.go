package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type MiscHandler struct{}

func (h *MiscHandler) Visibility(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
