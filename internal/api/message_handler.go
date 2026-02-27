package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

	text := req.Text
	if len(req.Attachments) > 0 {
		var b strings.Builder
		b.WriteString(text)
		b.WriteString("\n\nAttached files:\n")
		for _, att := range req.Attachments {
			if att.Path == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s (%s)\n", att.Filename, att.Path))
		}
		text = b.String()
	}

	if h.Mgr != nil {
		if !h.Mgr.HasAgent(sessionID) {
			sess, err := h.Store.GetSession(sessionID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if sess == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
				return
			}

			dir := "."
			flavor := "claude"
			if sess.Metadata != nil {
				if sess.Metadata.Path != "" {
					dir = sess.Metadata.Path
				}
				if sess.Metadata.Flavor != "" {
					flavor = sess.Metadata.Flavor
				}
			}

			startSeq, countErr := h.Store.GetMessageCount(sessionID)
			if countErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": countErr.Error()})
				return
			}
			agentSID := ""
			if sess.Metadata != nil {
				switch flavor {
				case "claude":
					agentSID = sess.Metadata.ClaudeSessionID
				case "codex":
					agentSID = sess.Metadata.CodexSessionID
				}
			}
			h.Mgr.SpawnAgentWithSession(sessionID, session.CreateSessionRequest{
				Directory: dir,
				Agent:     flavor,
				Model:     sess.ModelMode,
			}, startSeq, agentSID)
			_ = h.Store.SetSessionActive(sessionID, true)
			if h.Broker != nil {
				h.Broker.Publish(session.SyncEvent{
					Type: "session-updated", SessionID: sessionID,
				})
			}
		}

		if err := h.Mgr.SendMessage(sessionID, text); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
