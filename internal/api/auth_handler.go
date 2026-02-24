package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/liangzd/hapi-lite/internal/auth"
	"github.com/liangzd/hapi-lite/internal/config"
)

func AuthHandler(c *gin.Context) {
	var body struct {
		AccessToken string `json:"accessToken"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.AccessToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accessToken is required"})
		return
	}

	if subtle.ConstantTimeCompare([]byte(body.AccessToken), []byte(config.C.AccessToken)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid access token"})
		return
	}

	token, err := auth.GenerateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  gin.H{"id": 1, "firstName": "Web User"},
	})
}
