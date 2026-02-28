package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/liangzd/hapi-lite/internal/scanner"
)

type Manager struct {
	mu                  sync.RWMutex
	agents              map[string]*AgentProcess
	onEvent             func(sessionID string, event SyncEvent)
	onMessage           func(sessionID string, msg Message)
	onUpdateAgentSessId func(sessionID, agentSessionID, agent string) // 持久化 agentSessionID 到 DB
}

type AgentProcess struct {
	AgentSessionID string
	Req            CreateSessionRequest
	Scanner        scanner.Scanner
	Running        bool
	RunningAt      int64
	seq            int64
	cancel         context.CancelFunc
}

func NewManager(onEvent func(string, SyncEvent), onMessage func(string, Message), onUpdateAgentSessId func(string, string, string)) *Manager {
	return &Manager{
		agents:              make(map[string]*AgentProcess),
		onEvent:             onEvent,
		onMessage:           onMessage,
		onUpdateAgentSessId: onUpdateAgentSessId,
	}
}

func (m *Manager) SpawnAgent(sessionID string, req CreateSessionRequest, startSeq int64) {
	m.SpawnAgentWithSession(sessionID, req, startSeq, "")
}

func (m *Manager) SpawnAgentWithSession(sessionID string, req CreateSessionRequest, startSeq int64, agentSessionID string) {
	if req.Agent == "" {
		req.Agent = "claude"
	}
	if startSeq < 0 {
		startSeq = 0
	}
	proc := &AgentProcess{Req: req, seq: startSeq, AgentSessionID: agentSessionID}

	// Gemini/OpenCode still use file-based scanners.
	// Codex uses `codex exec --json` parsing in-process.
	if req.Agent != "claude" && req.Agent != "codex" {
		msgCb := func(sid string, sm scanner.ScannedMessage) {
			m.emitMessage(sid, proc, sm.Content, sm.CreatedAt)
		}
		var sc scanner.Scanner
		switch req.Agent {
		case "gemini":
			sc = scanner.NewGeminiScanner(msgCb)
		case "opencode":
			sc = scanner.NewOpencodeScanner(msgCb)
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
	if proc.cancel != nil {
		proc.cancel()
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

func (m *Manager) AbortAgent(sessionID string) {
	m.mu.RLock()
	proc, ok := m.agents[sessionID]
	m.mu.RUnlock()
	if ok && proc.cancel != nil {
		proc.cancel()
	}
}

func (m *Manager) IsRunning(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	proc, ok := m.agents[sessionID]
	if !ok {
		return false
	}
	return proc.Running
}

func (m *Manager) RunningAt(sessionID string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	proc, ok := m.agents[sessionID]
	if !ok {
		return 0
	}
	return proc.RunningAt
}

func (m *Manager) SendMessage(sessionID string, text string) error {
	m.mu.Lock()
	proc, ok := m.agents[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no active agent for session %s", sessionID)
	}
	if proc.Running {
		m.mu.Unlock()
		return fmt.Errorf("agent is busy, please wait for the current message to complete")
	}

	// 拦截 /clear：清空 AgentSessionID，下次发消息开新 agent session
	if strings.TrimSpace(text) == "/clear" {
		proc.AgentSessionID = ""
		m.mu.Unlock()
		if m.onUpdateAgentSessId != nil {
			m.onUpdateAgentSessId(sessionID, "", proc.Req.Agent)
		}
		m.emitMessage(sessionID, proc, buildEventMessage("message", map[string]interface{}{"type": "message", "message": "Context was reset"}), time.Now().UnixMilli())
		if m.onEvent != nil {
			m.onEvent(sessionID, SyncEvent{Type: "session-updated", SessionID: sessionID})
		}
		return nil
	}

	proc.Running = true
	proc.RunningAt = time.Now().UnixMilli()
	ctx, cancel := context.WithCancel(context.Background())
	proc.cancel = cancel
	m.mu.Unlock()

	// Always persist user prompts to database for all agent types
	raw := buildRoleWrappedMessage("user", map[string]interface{}{
		"type": "text",
		"text": text,
	})
	m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())

	if m.onEvent != nil {
		m.onEvent(sessionID, SyncEvent{Type: "session-updated", SessionID: sessionID})
	}

	go m.runOnce(ctx, cancel, sessionID, proc, text)
	return nil
}

func (m *Manager) runOnce(ctx context.Context, cancel context.CancelFunc, sessionID string, proc *AgentProcess, text string) {
	defer func() {
		cancel()
		m.mu.Lock()
		proc.Running = false
		proc.RunningAt = 0
		proc.cancel = nil
		m.mu.Unlock()
		if m.onEvent != nil {
			m.onEvent(sessionID, SyncEvent{Type: "session-updated", SessionID: sessionID})
		}
	}()

	req := proc.Req
	cmd := m.buildCmd(ctx, sessionID, req, text)
	if cmd == nil {
		return
	}

	// For claude: capture stdout to parse stream-json
	if req.Agent == "claude" {
		m.runClaude(ctx, cmd, sessionID, proc)
		return
	}
	if req.Agent == "codex" {
		m.runCodex(ctx, cmd, sessionID, proc)
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
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("%s exited with error: %v", req.Agent, err)), time.Now().UnixMilli())
	}
}

func (m *Manager) buildCmd(ctx context.Context, sessionID string, req CreateSessionRequest, text string) *exec.Cmd {
	m.mu.RLock()
	agentSessionID := m.agents[sessionID].AgentSessionID
	m.mu.RUnlock()

	var cmd *exec.Cmd
	switch req.Agent {
	case "claude":
		// `--verbose` is required for stable stream-json event coverage
		// (including thinking-related assistant blocks in newer Claude CLI versions).
		args := []string{"--print", "--verbose", "--output-format", "stream-json"}
		if req.Yolo {
			args = append(args, "--dangerously-skip-permissions")
		}
		if req.Model != "" && req.Model != "default" {
			args = append(args, "--model", req.Model)
		}
		if agentSessionID != "" {
			args = append(args, "--resume", agentSessionID)
		} else {
			agentSessionID = uuid.New().String()
			m.mu.Lock()
			m.agents[sessionID].AgentSessionID = agentSessionID
			m.mu.Unlock()
			if m.onUpdateAgentSessId != nil {
				m.onUpdateAgentSessId(sessionID, agentSessionID, "claude")
			}
			args = append(args, "--session-id", agentSessionID)
		}
		args = append(args, text)
		cmd = exec.CommandContext(ctx, "claude", args...)
	case "codex":
		var args []string
		if agentSessionID != "" {
			args = []string{"exec", "resume", "--json", "--skip-git-repo-check"}
			if req.Yolo {
				args = append(args, "--full-auto")
			}
			if req.Model != "" && req.Model != "default" {
				args = append(args, "--model", req.Model)
			}
			args = append(args, agentSessionID, text)
		} else {
			args = []string{"exec", "--json", "--skip-git-repo-check"}
			if req.Yolo {
				args = append(args, "--full-auto")
			}
			if req.Model != "" && req.Model != "default" {
				args = append(args, "--model", req.Model)
			}
			args = append(args, text)
		}
		cmd = exec.CommandContext(ctx, "codex", args...)
	case "gemini":
		cmd = exec.CommandContext(ctx, "gemini", text)
	case "opencode":
		cmd = exec.CommandContext(ctx, "opencode", text)
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
	// 设置新进程组，abort 时可以杀整个进程树（包括孙进程）
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func (m *Manager) runClaude(ctx context.Context, cmd *exec.Cmd, sessionID string, proc *AgentProcess) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdout pipe error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to initialize Claude output: %v", err)), time.Now().UnixMilli())
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stderr pipe error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to initialize Claude error output: %v", err)), time.Now().UnixMilli())
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to start Claude: %v", err)), time.Now().UnixMilli())
		return
	}
	stopKill := hookCancelKillProcessGroup(ctx, cmd)
	defer stopKill()

	// Capture stderr in a goroutine
	var stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(os.Stderr, "Claude stderr: %s\n", line)
			stderrBuf.WriteString(line)
			stderrBuf.WriteString("\n")
		}
	}()

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
	wg.Wait() // Wait for stderr goroutine to finish

	// abort 时不上报错误
	if ctx.Err() != nil {
		return
	}

	// If there were stderr messages, send them to the frontend
	if stderrOutput := strings.TrimSpace(stderrBuf.String()); stderrOutput != "" {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("⚠️ Claude Error:\n%s", stderrOutput)), time.Now().UnixMilli())
	}
}

func (m *Manager) runCodex(ctx context.Context, cmd *exec.Cmd, sessionID string, proc *AgentProcess) {
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
	stopKill := hookCancelKillProcessGroup(ctx, cmd)
	defer stopKill()

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

		// 捕获 thread_id 用于后续 resume
		if eventType == "thread.started" {
			if threadID := asStringValue(event["thread_id"]); threadID != "" && proc.AgentSessionID == "" {
				m.mu.Lock()
				proc.AgentSessionID = threadID
				m.mu.Unlock()
				if m.onUpdateAgentSessId != nil {
					m.onUpdateAgentSessId(sessionID, threadID, "codex")
				}
			}
			continue
		}

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
		case "web_search":
			callID := firstStringValue(item["id"], item["call_id"], item["callId"])
			if callID == "" {
				continue
			}
			query := asStringValue(item["query"])
			action, _ := item["action"].(map[string]interface{})
			url := ""
			if action != nil {
				url = asStringValue(action["url"])
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
			// web_search 即完成，直接发一条已完成的 tool-call（带 result），避免分页截断导致 running 卡住
			raw := buildRoleWrappedMessage("agent", map[string]interface{}{
				"type": "codex",
				"data": map[string]interface{}{
					"type":   "tool-call",
					"name":   toolName,
					"callId": callID,
					"input":  input,
					"result": "done",
					"id":     uuid.New().String(),
				},
			})
			m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
			emitted = true
		case "command_execution":
			cmdStr := strings.TrimRight(asStringValue(item["command"]), "\r\n")
			if cmdStr == "" {
				continue
			}
			callID := firstStringValue(item["id"], item["call_id"])
			if callID == "" {
				callID = uuid.New().String()
			}
			output := strings.TrimRight(asStringValue(item["aggregated_output"]), "\r\n")
			exitCode := item["exit_code"]
			raw := buildRoleWrappedMessage("agent", map[string]interface{}{
				"type": "codex",
				"data": map[string]interface{}{
					"type":     "tool-call",
					"name":     "Shell",
					"callId":   callID,
					"input":    map[string]interface{}{"command": cmdStr},
					"result":   output,
					"exitCode": exitCode,
					"id":       uuid.New().String(),
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

	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("codex exited with error: %v", err)), time.Now().UnixMilli())
	} else if !emitted && ctx.Err() == nil {
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

func buildEventMessage(_ string, data interface{}) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"role":    "agent",
		"content": map[string]interface{}{"type": "event", "data": data},
	})
	return json.RawMessage(b)
}

func hookCancelKillProcessGroup(ctx context.Context, cmd *exec.Cmd) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killProcessGroup(cmd)
		case <-done:
		}
	}()
	return func() { close(done) }
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil && pgid > 0 {
		if killErr := syscall.Kill(-pgid, syscall.SIGKILL); killErr == nil || killErr == syscall.ESRCH {
			return
		}
	}

	_ = cmd.Process.Kill()
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
