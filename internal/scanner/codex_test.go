package scanner

import (
	"encoding/json"
	"testing"
)

func TestConvertCodexLineToMessages_UserMessage(t *testing.T) {
	line := []byte(`{"type":"event_msg","payload":{"type":"user_message","message":"hello world\n"}}`)
	msgs := convertCodexLineToMessages(line)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var wrapped struct {
		Role    string `json:"role"`
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(msgs[0], &wrapped); err != nil {
		t.Fatalf("unmarshal message failed: %v", err)
	}
	if wrapped.Role != "user" {
		t.Fatalf("expected role user, got %q", wrapped.Role)
	}
	if wrapped.Content.Type != "text" {
		t.Fatalf("expected content.type text, got %q", wrapped.Content.Type)
	}
	if wrapped.Content.Text != "hello world" {
		t.Fatalf("expected trimmed text, got %q", wrapped.Content.Text)
	}
}

func TestConvertCodexLineToMessages_AgentMessage(t *testing.T) {
	line := []byte(`{"type":"event_msg","payload":{"type":"agent_message","message":"assistant reply"}}`)
	msgs := convertCodexLineToMessages(line)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var wrapped map[string]interface{}
	if err := json.Unmarshal(msgs[0], &wrapped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if wrapped["role"] != "agent" {
		t.Fatalf("expected role agent, got %#v", wrapped["role"])
	}
	content, _ := wrapped["content"].(map[string]interface{})
	if content["type"] != "codex" {
		t.Fatalf("expected content.type codex, got %#v", content["type"])
	}
	data, _ := content["data"].(map[string]interface{})
	if data["type"] != "message" {
		t.Fatalf("expected data.type message, got %#v", data["type"])
	}
	if data["message"] != "assistant reply" {
		t.Fatalf("expected data.message assistant reply, got %#v", data["message"])
	}
}

func TestConvertCodexLineToMessages_FunctionCall(t *testing.T) {
	line := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call_123","arguments":"{\"cmd\":\"ls -la\"}"}}`)
	msgs := convertCodexLineToMessages(line)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var wrapped map[string]interface{}
	if err := json.Unmarshal(msgs[0], &wrapped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	content, _ := wrapped["content"].(map[string]interface{})
	data, _ := content["data"].(map[string]interface{})
	if data["type"] != "tool-call" {
		t.Fatalf("expected tool-call, got %#v", data["type"])
	}
	if data["name"] != "exec_command" {
		t.Fatalf("expected name exec_command, got %#v", data["name"])
	}
	if data["callId"] != "call_123" {
		t.Fatalf("expected callId call_123, got %#v", data["callId"])
	}
	input, _ := data["input"].(map[string]interface{})
	if input["cmd"] != "ls -la" {
		t.Fatalf("expected parsed JSON input cmd, got %#v", input["cmd"])
	}
}

func TestConvertCodexLineToMessages_FunctionCallOutput(t *testing.T) {
	line := []byte(`{"type":"response_item","payload":{"type":"function_call_output","call_id":"call_789","output":"ok"}}`)
	msgs := convertCodexLineToMessages(line)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var wrapped map[string]interface{}
	if err := json.Unmarshal(msgs[0], &wrapped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	content, _ := wrapped["content"].(map[string]interface{})
	data, _ := content["data"].(map[string]interface{})
	if data["type"] != "tool-call-result" {
		t.Fatalf("expected tool-call-result, got %#v", data["type"])
	}
	if data["callId"] != "call_789" {
		t.Fatalf("expected callId call_789, got %#v", data["callId"])
	}
	if data["output"] != "ok" {
		t.Fatalf("expected output ok, got %#v", data["output"])
	}
}
