package codex

import (
	"encoding/json"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

func (p *Pipeline) After(_ pipeline.SendInput, line []byte, ctx pipeline.ParseContext) []pipeline.Action {
	var event map[string]interface{}
	if json.Unmarshal(line, &event) != nil {
		return nil
	}
	handler, ok := p.afterMap[asString(event["type"])]
	if !ok || handler == nil {
		return nil
	}
	return handler(event, ctx)
}

func (p *Pipeline) handleThreadStarted(event map[string]interface{}, _ pipeline.ParseContext) []pipeline.Action {
	threadID := asString(event["thread_id"])
	if threadID == "" {
		return nil
	}
	return []pipeline.Action{{
		Kind:           pipeline.ActionBindSessionID,
		AgentSessionID: threadID,
	}}
}

func (p *Pipeline) handleItemCompleted(event map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	item, _ := event["item"].(map[string]interface{})
	if item == nil {
		return nil
	}
	itemType := normalizeCodexItemType(asString(item["type"]))
	handler, ok := p.afterItemMap[itemType]
	if !ok || handler == nil {
		return nil
	}
	return handler(item, ctx)
}

func emitCodexData(data map[string]interface{}) []pipeline.Action {
	return []pipeline.Action{{
		Kind: pipeline.ActionEmitMessage,
		Message: pipeline.BuildRoleWrappedMessage("agent", map[string]interface{}{
			"type": "codex",
			"data": data,
		}),
	}}
}

func codexMsgID(ctx pipeline.ParseContext) string {
	if ctx.NewID == nil {
		return ""
	}
	return ctx.NewID()
}
