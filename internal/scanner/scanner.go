package scanner

import "encoding/json"

// Scanner watches agent session files and emits messages
type Scanner interface {
	Start(sessionID string, agentSessionID string, dir string) error
	Stop()
}

type ScannedMessage struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Seq       int64           `json:"seq"`
	Content   json.RawMessage `json:"content"`
	CreatedAt int64           `json:"createdAt"`
}

type MessageCallback func(sessionID string, msg ScannedMessage)
type StateCallback func(sessionID string, state json.RawMessage)
