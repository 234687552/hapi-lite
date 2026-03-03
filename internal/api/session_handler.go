package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/liangzd/hapi-lite/internal/session"
)

type SessionHandler struct {
	*BaseHandler
}

func (h *SessionHandler) publishStateChange(sessionID string, reason string) {
	if h.Broker == nil {
		return
	}

	snap := session.RuntimeSnapshot{State: session.RuntimeStateInactive}
	if h.Mgr != nil {
		snap = h.Mgr.RuntimeSnapshot(sessionID)
	}
	if snap.State == session.RuntimeStateInactive && h.Store != nil {
		if sess, err := h.Store.GetSession(sessionID); err == nil && sess != nil && sess.Active {
			snap.State = session.RuntimeStateReady
		}
	}

	h.Broker.Publish(session.SyncEvent{
		Type:      session.SyncEventSessionStateChange,
		SessionID: sessionID,
		Data: session.SessionStateChangeData{
			State:     snap.State,
			RunningAt: snap.RunningAt,
			Reason:    reason,
		},
	})
}

func (h *SessionHandler) hydrateRuntimeState(sess *session.Session) {
	if sess == nil || h.Mgr == nil {
		return
	}

	if h.Mgr.IsRunning(sess.ID) {
		sess.Thinking = true
		if startedAt := h.Mgr.RunningAt(sess.ID); startedAt > 0 {
			sess.ThinkingAt = startedAt
		} else if sess.ThinkingAt <= 0 {
			sess.ThinkingAt = sess.UpdatedAt
		}
		return
	}

	sess.Thinking = false
	sess.ThinkingAt = 0
}

func toSummary(sess session.Session) session.SessionSummary {
	var meta *session.SessionSummaryMetadata
	if sess.Metadata != nil {
		meta = &session.SessionSummaryMetadata{
			Name:     sess.Metadata.Name,
			Path:     sess.Metadata.Path,
			Flavor:   sess.Metadata.Flavor,
			Worktree: sess.Metadata.Worktree,
		}
	}
	if meta != nil && meta.Path == "" {
		meta.Path = "."
	}

	pending := 0
	if sess.AgentState != nil && sess.AgentState.Requests != nil {
		pending = len(sess.AgentState.Requests)
	}

	return session.SessionSummary{
		ID:                   sess.ID,
		Active:               sess.Active,
		Thinking:             sess.Thinking,
		ActiveAt:             sess.ActiveAt,
		UpdatedAt:            sess.UpdatedAt,
		Metadata:             meta,
		PendingRequestsCount: pending,
		ModelMode:            sess.ModelMode,
	}
}

func (h *SessionHandler) List(c *gin.Context) {
	sessions, err := h.Store.GetSessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	summaries := make([]session.SessionSummary, 0, len(sessions))
	for _, sess := range sessions {
		h.hydrateRuntimeState(&sess)
		summaries = append(summaries, toSummary(sess))
	}
	c.JSON(http.StatusOK, gin.H{"sessions": summaries})
}

func (h *SessionHandler) Get(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.Store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}
	h.hydrateRuntimeState(sess)
	c.JSON(http.StatusOK, gin.H{"session": sess})
}

func (h *SessionHandler) Create(c *gin.Context) {
	var req session.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}
	if req.Agent == "" {
		req.Agent = string(session.FlavorClaude)
	}

	sess, err := h.Store.CreateSession(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Spawn agent process
	if h.Mgr != nil {
		startSeq, countErr := h.Store.GetMessageCount(sess.ID)
		if countErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": countErr.Error()})
			return
		}
		h.Mgr.SpawnAgent(sess.ID, req, startSeq)
	}

	h.Broker.Publish(session.SyncEvent{
		Type: "session-added", SessionID: sess.ID,
	})
	h.publishStateChange(sess.ID, "created")

	c.JSON(http.StatusOK, gin.H{"sessionId": sess.ID, "session": sess})
}

func (h *SessionHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	sess, _ := h.Store.GetSession(id)
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}
	if sess.Active {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete active session. Archive it first."})
		return
	}

	if h.Mgr != nil {
		h.Mgr.StopAgent(id)
	}
	h.Store.DeleteSession(id)

	h.Broker.Publish(session.SyncEvent{
		Type: "session-removed", SessionID: id,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SessionHandler) Abort(c *gin.Context) {
	id := c.Param("id")
	if h.Mgr != nil {
		h.Mgr.AbortAgent(id)
	}
	h.publishStateChange(id, "abort-requested")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SessionHandler) SetPermissionMode(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Mode string `json:"mode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}
	if h.Mgr != nil {
		h.Mgr.SetPermissionMode(id, body.Mode)
	}
	_ = h.Store.SetSessionPermissionMode(id, body.Mode)
	h.publishStateChange(id, "permission-mode-changed")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SessionHandler) SetModel(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Model string `json:"model" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}
	if h.Mgr != nil {
		h.Mgr.SetModelMode(id, body.Model)
	}
	_ = h.Store.SetSessionModelMode(id, body.Model)
	h.publishStateChange(id, "model-changed")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SessionHandler) Resume(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.Store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found", "code": "session_not_found"})
		return
	}
	if sess.Active {
		c.JSON(http.StatusOK, gin.H{"type": "success", "sessionId": id})
		return
	}

	dir := "."
	flavor := string(session.FlavorClaude)
	if sess.Metadata != nil {
		if sess.Metadata.Path != "" {
			dir = sess.Metadata.Path
		}
		if sess.Metadata.Flavor != "" {
			flavor = sess.Metadata.Flavor
		}
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Session directory no longer exists", "code": "resume_unavailable"})
		return
	}

	req := session.CreateSessionRequest{
		Directory: dir,
		Agent:     flavor,
		Model:     sess.ModelMode,
	}
	if h.Mgr != nil {
		startSeq, countErr := h.Store.GetMessageCount(id)
		if countErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": countErr.Error()})
			return
		}
		agentSID := session.GetAgentSessionID(sess.Metadata, flavor)
		h.Mgr.SpawnAgentWithSession(id, req, startSeq, agentSID)
	}
	_ = h.Store.SetSessionActive(id, true)
	h.publishStateChange(id, "resumed")

	c.JSON(http.StatusOK, gin.H{"type": "success", "sessionId": id})
}

func (h *SessionHandler) Archive(c *gin.Context) {
	id := c.Param("id")
	if h.Mgr != nil {
		h.Mgr.StopAgent(id)
	}
	_ = h.Store.SetSessionActive(id, false)
	h.publishStateChange(id, "archived")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SessionHandler) Rename(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body: name is required"})
		return
	}
	if err := h.Store.RenameSession(id, strings.TrimSpace(body.Name)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.publishStateChange(id, "renamed")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *SessionHandler) ListSlashCommands(c *gin.Context) {
	id := c.Param("id")
	sess, err := h.Store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	flavor := string(session.FlavorClaude)
	if sess.Metadata != nil && sess.Metadata.Flavor != "" {
		flavor = sess.Metadata.Flavor
	}

	rawCommands := session.SlashCommandsForAgent(flavor)
	commands := make([]gin.H, 0, len(rawCommands))
	for _, cmd := range rawCommands {
		commands = append(commands, gin.H{
			"name":        cmd.Name,
			"description": cmd.Description,
			"source":      cmd.Source,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "commands": commands})
}

func (h *SessionHandler) ListSkills(c *gin.Context) {
	home, _ := os.UserHomeDir()
	skillsDir := filepath.Join(home, ".codex", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "skills": []gin.H{}})
		return
	}

	type skillItem struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	items := make([]skillItem, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		desc := ""
		if data, err := os.ReadFile(skillFile); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				t := strings.TrimSpace(line)
				if t == "" || strings.HasPrefix(t, "#") {
					continue
				}
				desc = t
				break
			}
		}
		items = append(items, skillItem{
			Name:        entry.Name(),
			Description: desc,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "skills": items})
}
