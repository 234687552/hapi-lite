package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/liangzd/hapi-lite/internal/session"
)

type MessageHandler struct {
	*BaseHandler
}

func (h *MessageHandler) List(c *gin.Context) {
	sessionID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var beforeSeq *int64
	if bs := c.Query("beforeSeq"); bs != "" {
		v, _ := strconv.ParseInt(bs, 10, 64)
		if v > 0 {
			beforeSeq = &v
		}
	}

	msgs, err := h.Store.GetMessages(sessionID, limit, beforeSeq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []session.Message{}
	}

	var total int64
	if beforeSeq != nil {
		total, err = h.Store.GetMessageCountBefore(sessionID, *beforeSeq)
	} else {
		total, err = h.Store.GetMessageCount(sessionID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hasMore := int64(len(msgs)) < total
	var nextBeforeSeq *int64
	if len(msgs) > 0 {
		v := msgs[0].Seq
		nextBeforeSeq = &v
	}

	var beforeSeqValue interface{}
	if beforeSeq != nil {
		beforeSeqValue = *beforeSeq
	}

	c.JSON(http.StatusOK, gin.H{
		"messages": msgs,
		"page": gin.H{
			"limit":         limit,
			"beforeSeq":     beforeSeqValue,
			"nextBeforeSeq": nextBeforeSeq,
			"hasMore":       hasMore,
		},
	})
}

func (h *MessageHandler) Send(c *gin.Context) {
	sessionID := c.Param("id")
	var req session.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}

	svc := NewSendService(h.Store, h.Broker, h.Mgr)
	result, err := svc.Send(sessionID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		case errors.Is(err, session.ErrAgentBusy):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusAccepted, result)
}
