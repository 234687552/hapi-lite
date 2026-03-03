package session

const (
	SyncEventMessageAppended    = "message-appended"
	SyncEventSessionStateChange = "session-state-changed"
)

type SessionStateChangeData struct {
	State     RuntimeState `json:"state"`
	RunningAt int64        `json:"runningAt,omitempty"`
	Reason    string       `json:"reason,omitempty"`
}
