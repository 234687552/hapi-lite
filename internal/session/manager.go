package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/liangzd/hapi-lite/internal/scanner"
)

type Manager struct {
	mu        sync.RWMutex
	agents    map[string]*AgentProcess
	onEvent   func(sessionID string, event SyncEvent)
	onMessage func(sessionID string, msg Message)
	onExit    func(sessionID string)
}

type AgentProcess struct {
	SessionID string
	Req       CreateSessionRequest
	Scanner   scanner.Scanner
	HasSent   bool
	Running   bool
	seq       int64
}

func NewManager(onEvent func(string, SyncEvent), onMessage func(string, Message), onExit func(string)) *Manager {
	return &Manager{
		agents:    make(map[string]*AgentProcess),
		onEvent:   onEvent,
		onMessage: onMessage,
		onExit:    onExit,
	}
}

func (m *Manager) SpawnAgent(sessionID string, req CreateSessionRequest, startSeq int64) {
	if req.Agent == "" {
		req.Agent = "claude"
	}
	if startSeq < 0 {
		startSeq = 0
	}

	proc := &AgentProcess{SessionID: sessionID, Req: req, seq: startSeq}

	// Gemini/OpenCode still use file-based scanners.
	// Codex uses `codex exec --json` parsing in-process.
	if req.Agent != "claude" && req.Agent != "codex" {
		msgCb := func(sid string, sm scanner.ScannedMessage) {
			m.emitMessage(sid, proc, sm.Content, sm.CreatedAt)
		}
		var sc scanner.Scanner
		switch req.Agent {
		case "gemini":
			sc = scanner.NewGeminiScanner(msgCb, nil)
		case "opencode":
			sc = scanner.NewOpencodeScanner(msgCb, nil)
		}
		proc.Scanner = sc
		if sc != nil {
			sc.Start(sessionID, "", req.Directory)
		}
	}

	m.mu.Lock()
	m.agents[sessionID] = proc
	m.mu.Unlock()
}

func (m *Manager) StopAgent(sessionID string) {
	m.mu.RLock()
	proc, ok := m.agents[sessionID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	if proc.Scanner != nil {
		proc.Scanner.Stop()
	}
	m.mu.Lock()
	delete(m.agents, sessionID)
	m.mu.Unlock()
}

func (m *Manager) HasAgent(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.agents[sessionID]
	return ok
}

func (m *Manager) AbortAgent(sessionID string) { m.StopAgent(sessionID) }

func (m *Manager) SendMessage(sessionID string, text string) error {
	m.mu.RLock()
	proc, ok := m.agents[sessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no active agent for session %s", sessionID)
	}
	if proc.Running {
		return fmt.Errorf("agent is busy")
	}

	isContinue := proc.HasSent
	proc.HasSent = true
	proc.Running = true

	if proc.Req.Agent == "codex" {
		// codex exec output does not emit user prompts, so persist it explicitly.
		raw := buildRoleWrappedMessage("user", map[string]interface{}{
			"type": "text",
			"text": text,
		})
		m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
	}

	go m.runOnce(sessionID, proc, text, isContinue)
	return nil
}

func (m *Manager) runOnce(sessionID string, proc *AgentProcess, text string, isContinue bool) {
	defer func() {
		proc.Running = false
		if m.onEvent != nil {
			m.onEvent(sessionID, SyncEvent{Type: "session-updated", SessionID: sessionID})
		}
	}()

	req := proc.Req
	cmd := m.buildCmd(req, text, isContinue)
	if cmd == nil {
		return
	}

	// For claude: capture stdout to parse stream-json
	if req.Agent == "claude" {
		m.runClaude(cmd, sessionID, proc)
		return
	}
	if req.Agent == "codex" {
		m.runCodex(cmd, sessionID, proc)
		return
	}

	// Other agents: just run, scanner handles output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to start %s: %v", req.Agent, err)), time.Now().UnixMilli())
		return
	}
	if err := cmd.Wait(); err != nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("%s exited with error: %v", req.Agent, err)), time.Now().UnixMilli())
	}
}

func (m *Manager) buildCmd(req CreateSessionRequest, text string, isContinue bool) *exec.Cmd {
	var cmd *exec.Cmd
	switch req.Agent {
	case "claude":
		args := []string{"--print", "--output-format", "stream-json"}
		if req.Yolo {
			args = append(args, "--dangerously-skip-permissions")
		}
		if req.Model != "" && req.Model != "default" {
			args = append(args, "--model", req.Model)
		}
		if isContinue {
			args = append(args, "--continue")
		}
		args = append(args, text)
		cmd = exec.Command("claude", args...)
	case "codex":
		args := []string{"exec", "--json", "--skip-git-repo-check"}
		if req.Yolo {
			args = append(args, "--full-auto")
		}
		if req.Model != "" && req.Model != "default" {
			args = append(args, "--model", req.Model)
		}
		args = append(args, text)
		cmd = exec.Command("codex", args...)
	case "gemini":
		cmd = exec.Command("gemini", text)
	case "opencode":
		cmd = exec.Command("opencode", text)
	default:
		return nil
	}
	cmd.Dir = req.Directory
	// Filter out CLAUDECODE env var to avoid nested session detection
	var env []string
	for _, e := range os.Environ() {
		if len(e) > 10 && e[:10] == "CLAUDECODE" {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "TERM=dumb")
	return cmd
}

func (m *Manager) runClaude(cmd *exec.Cmd, sessionID string, proc *AgentProcess) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdout pipe error: %v\n", err)
		return
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
		return
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var peek struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &peek) != nil {
			continue
		}
		if peek.Type != "user" && peek.Type != "assistant" && peek.Type != "summary" {
			continue
		}

		raw := json.RawMessage(append([]byte{}, line...))
		m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
	}

	cmd.Wait()
}

func (m *Manager) runCodex(cmd *exec.Cmd, sessionID string, proc *AgentProcess) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdout pipe error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to initialize codex output: %v", err)), time.Now().UnixMilli())
		return
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to start codex: %v", err)), time.Now().UnixMilli())
		return
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 128*1024), 8*1024*1024)
	emitted := false

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var event map[string]interface{}
		if json.Unmarshal(line, &event) != nil {
			continue
		}

		eventType := asStringValue(event["type"])
		if eventType != "item.completed" {
			continue
		}
		item, _ := event["item"].(map[string]interface{})
		itemType := asStringValue(item["type"])
		switch itemType {
		case "agent_message":
			text := strings.TrimRight(asStringValue(item["text"]), "\r\n")
			if text == "" {
				continue
			}
			raw := buildRoleWrappedMessage("agent", map[string]interface{}{
				"type": "codex",
				"data": map[string]interface{}{
					"type":    "message",
					"message": text,
					"id":      uuid.New().String(),
				},
			})
			m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
			emitted = true
		case "reasoning":
			text := strings.TrimRight(asStringValue(item["text"]), "\r\n")
			if text == "" {
				continue
			}
			raw := buildRoleWrappedMessage("agent", map[string]interface{}{
				"type": "codex",
				"data": map[string]interface{}{
					"type":    "reasoning",
					"message": text,
					"id":      uuid.New().String(),
				},
			})
			m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
			emitted = true
		case "tool_call", "tool-call", "function_call":
			callID := firstStringValue(item["call_id"], item["callId"], item["tool_call_id"], item["toolCallId"], item["id"])
			name := asStringValue(item["name"])
			if callID == "" || name == "" {
				continue
			}
			input := item["input"]
			if input == nil {
				input = parseMaybeJSONValue(item["arguments"])
			}
			raw := buildRoleWrappedMessage("agent", map[string]interface{}{
				"type": "codex",
				"data": map[string]interface{}{
					"type":   "tool-call",
					"name":   name,
					"callId": callID,
					"input":  input,
					"id":     uuid.New().String(),
				},
			})
			m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
			emitted = true
		case "tool_result", "tool-call-result", "function_call_output":
			callID := firstStringValue(item["call_id"], item["callId"], item["tool_call_id"], item["toolCallId"], item["id"])
			if callID == "" {
				continue
			}
			output := item["output"]
			if output == nil {
				output = item["content"]
			}
			raw := buildRoleWrappedMessage("agent", map[string]interface{}{
				"type": "codex",
				"data": map[string]interface{}{
					"type":   "tool-call-result",
					"callId": callID,
					"output": output,
					"id":     uuid.New().String(),
				},
			})
			m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
			emitted = true
		}
	}

	if err := cmd.Wait(); err != nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("codex exited with error: %v", err)), time.Now().UnixMilli())
	} else if !emitted {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage("codex finished without response content"), time.Now().UnixMilli())
	}
}

func (m *Manager) emitMessage(sessionID string, proc *AgentProcess, raw json.RawMessage, createdAt int64) {
	proc.seq++
	msg := Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Seq:       proc.seq,
		Content:   raw,
		CreatedAt: createdAt,
	}
	if m.onMessage != nil {
		m.onMessage(sessionID, msg)
	}
	if m.onEvent != nil {
		m.onEvent(sessionID, SyncEvent{
			Type: "message-received", SessionID: sessionID, Message: &msg,
		})
	}
}

func buildRoleWrappedMessage(role string, content interface{}) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"role":    role,
		"content": content,
	})
	return json.RawMessage(b)
}

func buildAssistantTextMessage(text string) json.RawMessage {
	return buildRoleWrappedMessage("assistant", text)
}

func asStringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

func firstStringValue(values ...interface{}) string {
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func parseMaybeJSONValue(v interface{}) interface{} {
	s, ok := v.(string)
	if !ok {
		return v
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed interface{}
		if json.Unmarshal([]byte(trimmed), &parsed) == nil {
			return parsed
		}
	}
	return s
}

func (m *Manager) SetPermissionMode(sessionID string, mode string) {}
func (m *Manager) SetModelMode(sessionID string, model string)     {}
func (m *Manager) ApprovePermission(sessionID, requestID, decision string, answers interface{}) {
}
func (m *Manager) DenyPermission(sessionID, requestID string) {}
