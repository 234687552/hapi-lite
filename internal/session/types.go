package session

import "encoding/json"

type AgentFlavor string

const (
	FlavorClaude   AgentFlavor = "claude"
	FlavorCodex    AgentFlavor = "codex"
	FlavorGemini   AgentFlavor = "gemini"
	FlavorOpencode AgentFlavor = "opencode"
)

type AgentStateRequest struct {
	Tool      string      `json:"tool"`
	Arguments interface{} `json:"arguments"`
	CreatedAt *int64      `json:"createdAt,omitempty"`
}

type AgentStateCompletedRequest struct {
	Tool        string      `json:"tool"`
	Arguments   interface{} `json:"arguments"`
	CreatedAt   *int64      `json:"createdAt,omitempty"`
	CompletedAt *int64      `json:"completedAt,omitempty"`
	Status      string      `json:"status"`
	Reason      string      `json:"reason,omitempty"`
	Mode        string      `json:"mode,omitempty"`
	Decision    string      `json:"decision,omitempty"`
	AllowTools  []string    `json:"allowTools,omitempty"`
	Answers     interface{} `json:"answers,omitempty"`
}

type AgentState struct {
	Requests          map[string]AgentStateRequest          `json:"requests,omitempty"`
	CompletedRequests map[string]AgentStateCompletedRequest `json:"completedRequests,omitempty"`
}

type WorktreeMetadata struct {
	BasePath     string `json:"basePath"`
	Branch       string `json:"branch"`
	Name         string `json:"name"`
	WorktreePath string `json:"worktreePath,omitempty"`
	CreatedAt    *int64 `json:"createdAt,omitempty"`
}

type Metadata struct {
	Path              string            `json:"path"`
	Host              string            `json:"host"`
	Version           string            `json:"version,omitempty"`
	Name              string            `json:"name,omitempty"`
	OS                string            `json:"os,omitempty"`
	Flavor            string            `json:"flavor,omitempty"`
	ClaudeSessionID   string            `json:"claudeSessionId,omitempty"`
	CodexSessionID    string            `json:"codexSessionId,omitempty"`
	GeminiSessionID   string            `json:"geminiSessionId,omitempty"`
	OpencodeSessionID string            `json:"opencodeSessionId,omitempty"`
	Tools             []string          `json:"tools,omitempty"`
	HomeDir           string            `json:"homeDir,omitempty"`
	Worktree          *WorktreeMetadata `json:"worktree,omitempty"`
}

func GetAgentSessionID(meta *Metadata, agent string) string {
	if meta == nil {
		return ""
	}
	switch agent {
	case string(FlavorClaude):
		return meta.ClaudeSessionID
	case string(FlavorCodex):
		return meta.CodexSessionID
	case string(FlavorGemini):
		return meta.GeminiSessionID
	case string(FlavorOpencode):
		return meta.OpencodeSessionID
	default:
		return ""
	}
}

func SetAgentSessionID(meta *Metadata, agent, value string) bool {
	if meta == nil {
		return false
	}
	switch agent {
	case string(FlavorClaude):
		meta.ClaudeSessionID = value
	case string(FlavorCodex):
		meta.CodexSessionID = value
	case string(FlavorGemini):
		meta.GeminiSessionID = value
	case string(FlavorOpencode):
		meta.OpencodeSessionID = value
	default:
		return false
	}
	return true
}

type Session struct {
	ID             string      `json:"id"`
	Seq            int64       `json:"seq"`
	CreatedAt      int64       `json:"createdAt"`
	UpdatedAt      int64       `json:"updatedAt"`
	Active         bool        `json:"active"`
	ActiveAt       int64       `json:"activeAt"`
	Metadata       *Metadata   `json:"metadata"`
	AgentState     *AgentState `json:"agentState"`
	Thinking       bool        `json:"thinking"`
	ThinkingAt     int64       `json:"thinkingAt"`
	PermissionMode string      `json:"permissionMode,omitempty"`
	ModelMode      string      `json:"modelMode,omitempty"`
}

type Message struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Seq       int64           `json:"seq"`
	LocalID   string          `json:"localId,omitempty"`
	Content   json.RawMessage `json:"content"`
	CreatedAt int64           `json:"createdAt"`
}

type AttachmentMetadata struct {
	ID         string `json:"id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
	Path       string `json:"path"`
	PreviewURL string `json:"previewUrl,omitempty"`
}

type SessionSummaryMetadata struct {
	Name      string            `json:"name,omitempty"`
	Path      string            `json:"path"`
	MachineID string            `json:"machineId,omitempty"`
	Flavor    string            `json:"flavor,omitempty"`
	Worktree  *WorktreeMetadata `json:"worktree,omitempty"`
}

type TodoProgress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

type SessionSummary struct {
	ID                   string                  `json:"id"`
	Active               bool                    `json:"active"`
	Thinking             bool                    `json:"thinking"`
	ActiveAt             int64                   `json:"activeAt"`
	UpdatedAt            int64                   `json:"updatedAt"`
	Metadata             *SessionSummaryMetadata `json:"metadata"`
	TodoProgress         *TodoProgress           `json:"todoProgress"`
	PendingRequestsCount int                     `json:"pendingRequestsCount"`
	ModelMode            string                  `json:"modelMode,omitempty"`
}

type CreateSessionRequest struct {
	Directory string `json:"directory" binding:"required"`
	Agent     string `json:"agent"`
	Model     string `json:"model,omitempty"`
	Yolo      bool   `json:"yolo,omitempty"`
}

type SendMessageRequest struct {
	Text        string               `json:"text" binding:"required"`
	LocalID     string               `json:"localId,omitempty"`
	Attachments []AttachmentMetadata `json:"attachments,omitempty"`
}

type SyncEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId,omitempty"`
	Message   *Message    `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}
