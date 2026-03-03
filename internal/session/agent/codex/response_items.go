package codex

import (
	"strings"

	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

func handleAgentMessageItem(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	text := strings.TrimRight(asString(item["text"]), "\r\n")
	if text == "" {
		return nil
	}
	return emitCodexData(map[string]interface{}{
		"type":    "message",
		"message": text,
		"id":      codexMsgID(ctx),
	})
}

func handleReasoningItem(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	text := strings.TrimRight(asString(item["text"]), "\r\n")
	if text == "" {
		return nil
	}
	return emitCodexData(map[string]interface{}{
		"type":    "reasoning",
		"message": text,
		"id":      codexMsgID(ctx),
	})
}

func handleToolCallItem(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	callID := firstString(item["call_id"], item["callId"], item["tool_call_id"], item["toolCallId"], item["id"])
	name := asString(item["name"])
	if callID == "" || name == "" {
		return nil
	}
	input := item["input"]
	if input == nil {
		input = parseMaybeJSONValue(item["arguments"])
	}
	return emitCodexData(map[string]interface{}{
		"type":   "tool-call",
		"name":   name,
		"callId": callID,
		"input":  input,
		"id":     codexMsgID(ctx),
	})
}

func handleWebSearchItem(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	callID := firstString(item["id"], item["call_id"], item["callId"])
	if callID == "" {
		return nil
	}
	query := asString(item["query"])
	action, _ := item["action"].(map[string]interface{})
	url := ""
	if action != nil {
		url = asString(action["url"])
	}
	input := map[string]interface{}{"query": query}
	if url != "" {
		input["url"] = url
	}
	toolName := "WebSearch"
	if url != "" && query == url {
		toolName = "WebFetch"
		input = map[string]interface{}{"url": url}
	}
	return emitCodexData(map[string]interface{}{
		"type":   "tool-call",
		"name":   toolName,
		"callId": callID,
		"input":  input,
		"result": "done",
		"id":     codexMsgID(ctx),
	})
}

func handleCommandExecutionItem(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	cmdStr := strings.TrimRight(asString(item["command"]), "\r\n")
	if cmdStr == "" {
		return nil
	}
	callID := firstString(item["id"], item["call_id"])
	if callID == "" {
		callID = codexMsgID(ctx)
	}
	output := strings.TrimRight(asString(item["aggregated_output"]), "\r\n")
	return emitCodexData(map[string]interface{}{
		"type":     "tool-call",
		"name":     "Shell",
		"callId":   callID,
		"input":    map[string]interface{}{"command": cmdStr},
		"result":   output,
		"exitCode": item["exit_code"],
		"id":       codexMsgID(ctx),
	})
}

func handleToolResultItem(item map[string]interface{}, ctx pipeline.ParseContext) []pipeline.Action {
	callID := firstString(item["call_id"], item["callId"], item["tool_call_id"], item["toolCallId"], item["id"])
	if callID == "" {
		return nil
	}
	output := item["output"]
	if output == nil {
		output = item["content"]
	}
	return emitCodexData(map[string]interface{}{
		"type":   "tool-call-result",
		"callId": callID,
		"output": output,
		"id":     codexMsgID(ctx),
	})
}
