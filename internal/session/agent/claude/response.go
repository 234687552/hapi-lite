package claude

import (
	"encoding/json"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

func (p *Pipeline) After(_ pipeline.SendInput, line []byte, ctx pipeline.ParseContext) []pipeline.Action {
	var peek struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(line, &peek) != nil {
		return nil
	}
	if !shouldEmitRawEnvelope(peek.Type) {
		return nil
	}
	return emitRawEnvelope(line, ctx)
}

func emitRawEnvelope(raw []byte, _ pipeline.ParseContext) []pipeline.Action {
	copied := append([]byte{}, raw...)
	return []pipeline.Action{{Kind: pipeline.ActionEmitMessage, Message: json.RawMessage(copied)}}
}

func shouldEmitRawEnvelope(eventType string) bool {
	return eventType == "user" || eventType == "assistant" || eventType == "summary"
}
