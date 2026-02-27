package api

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/liangzd/hapi-lite/internal/sse"
)

type SSEHandler struct {
	Broker *sse.Broker
}

func (h *SSEHandler) Events(c *gin.Context) {
	sessionID := c.Query("sessionId")
	if c.Query("all") == "true" || c.Query("all") == "1" {
		sessionID = ""
	}
	clientID := uuid.New().String()

	client := &sse.Client{
		ID:        clientID,
		SessionID: sessionID,
		Events:    make(chan string, 256),
	}
	h.Broker.Subscribe(client)
	defer h.Broker.Unsubscribe(clientID)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	// Send connection event
	c.Writer.WriteString("data: {\"type\":\"connection-changed\",\"data\":{\"status\":\"connected\",\"subscriptionId\":\"" + clientID + "\"}}\n\n")
	c.Writer.Flush()

	ctx := c.Request.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			writeHeartbeat(c.Writer)
		case msg, ok := <-client.Events:
			if !ok {
				return
			}
			io.WriteString(c.Writer, msg)
			c.Writer.Flush()
		}
	}
}

func writeHeartbeat(w http.ResponseWriter) {
	w.Write([]byte(": heartbeat\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
