package agent

import "encoding/json"

func ClearCommandBefore(input SendInput) BeforeResult {
	return contextResetBefore(input, "Context was reset")
}

func NewCommandBefore(input SendInput) BeforeResult {
	return contextResetBefore(input, "Started a new conversation")
}

func contextResetBefore(input SendInput, message string) BeforeResult {
	return BeforeResult{
		Input:      input,
		Stop:       true,
		StopReason: "context-reset",
		Actions: []Action{
			{Kind: ActionBindSessionID, AgentSessionID: "", Force: true},
			{Kind: ActionEmitMessage, Message: buildContextResetEvent(message)},
		},
	}
}

func buildContextResetEvent(message string) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"role": "agent",
		"content": map[string]interface{}{
			"type": "event",
			"data": map[string]interface{}{
				"type":    "message",
				"message": message,
			},
		},
	})
	return json.RawMessage(b)
}
