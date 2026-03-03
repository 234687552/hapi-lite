package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/liangzd/hapi-lite/internal/scanner"
	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
	"github.com/liangzd/hapi-lite/internal/session/agent/claude"
	"github.com/liangzd/hapi-lite/internal/session/agent/codex"
	"github.com/liangzd/hapi-lite/internal/session/agent/gemini"
	"github.com/liangzd/hapi-lite/internal/session/agent/opencode"
)

type Manager struct {
	mu                  sync.RWMutex
	agents              map[string]*AgentProcess
	runtimes            *RuntimeStore
	onEvent             func(sessionID string, event SyncEvent)
	onMessage           func(sessionID string, msg Message)
	onUpdateAgentSessId func(sessionID, agentSessionID, agent string) // 持久化 agentSessionID 到 DB
}

type AgentProcess struct {
	AgentSessionID string
	Req            CreateSessionRequest
	Scanner        scanner.Scanner
	seq            int64
	cancel         context.CancelFunc
}

type agentRuntime struct {
	name              string
	mode              pipeline.Mode
	stderrMode        pipeline.StderrMode
	emitExitError     bool
	noContentFallback string
	after             func(input pipeline.SendInput, line []byte, ctx pipeline.ParseContext) []pipeline.Action
}

var (
	ErrNoActiveAgent = errors.New("no active agent")
	ErrAgentBusy     = errors.New("agent is busy")
)

func NewManager(onEvent func(string, SyncEvent), onMessage func(string, Message), onUpdateAgentSessId func(string, string, string)) *Manager {
	return &Manager{
		agents:              make(map[string]*AgentProcess),
		runtimes:            NewRuntimeStore(),
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
		req.Agent = string(FlavorClaude)
	}
	if startSeq < 0 {
		startSeq = 0
	}
	proc := &AgentProcess{Req: req, seq: startSeq, AgentSessionID: agentSessionID}

	msgCb := func(sid string, sm scanner.ScannedMessage) {
		m.emitMessage(sid, proc, sm.Content, sm.CreatedAt)
	}
	var sc scanner.Scanner
	switch req.Agent {
	case string(FlavorGemini):
		sc = scanner.NewGeminiScanner(msgCb)
	case string(FlavorOpencode):
		sc = scanner.NewOpencodeScanner(msgCb)
	}
	proc.Scanner = sc
	if sc != nil {
		sc.Start(sessionID, "", req.Directory)
	}

	m.mu.Lock()
	m.agents[sessionID] = proc
	m.mu.Unlock()

	m.runtimes.Set(sessionID, RuntimeSnapshot{State: RuntimeStateReady})
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

	if _, err := m.runtimes.Transition(sessionID, RuntimeTransitionArchive, 0); err != nil {
		m.runtimes.Set(sessionID, RuntimeSnapshot{State: RuntimeStateInactive})
	}
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
	return m.runtimes.Get(sessionID).State == RuntimeStateRunning
}

func (m *Manager) RunningAt(sessionID string) int64 {
	return m.runtimes.Get(sessionID).RunningAt
}

func (m *Manager) RuntimeSnapshot(sessionID string) RuntimeSnapshot {
	return m.runtimes.Get(sessionID)
}

func (m *Manager) SendMessage(sessionID string, text string) (string, error) {
	m.mu.Lock()
	proc, ok := m.agents[sessionID]
	if !ok {
		m.mu.Unlock()
		return "", fmt.Errorf("%w for session %s", ErrNoActiveAgent, sessionID)
	}
	state := m.runtimes.Get(sessionID).State
	if state == RuntimeStateRunning {
		m.mu.Unlock()
		return "", fmt.Errorf("%w, please wait for the current message to complete", ErrAgentBusy)
	}
	if state != RuntimeStateReady {
		m.mu.Unlock()
		return "", fmt.Errorf("%w: session %s is %s", ErrInvalidRuntimeTransition, sessionID, state)
	}
	input := pipeline.SendInput{
		Request:        toPipelineRequest(proc.Req),
		Text:           text,
		AgentSessionID: proc.AgentSessionID,
	}
	var (
		agentInfo  agentRuntime
		sendResult pipeline.SendResult
		err        error
	)
	ctx, cancel := context.WithCancel(context.Background())
	switch proc.Req.Agent {
	case string(FlavorClaude):
		p := claude.New()
		agentInfo = agentRuntime{
			name:              p.Name(),
			mode:              p.Mode(),
			stderrMode:        p.StderrMode(),
			emitExitError:     p.EmitExitError(),
			noContentFallback: p.NoContentFallback(),
			after:             p.After,
		}
		sendResult, err = p.Send(ctx, input)
	case string(FlavorCodex):
		p := codex.New()
		agentInfo = agentRuntime{
			name:              p.Name(),
			mode:              p.Mode(),
			stderrMode:        p.StderrMode(),
			emitExitError:     p.EmitExitError(),
			noContentFallback: p.NoContentFallback(),
			after:             p.After,
		}
		sendResult, err = p.Send(ctx, input)
	case string(FlavorGemini):
		p := gemini.New()
		agentInfo = agentRuntime{
			name:              p.Name(),
			mode:              p.Mode(),
			stderrMode:        p.StderrMode(),
			emitExitError:     p.EmitExitError(),
			noContentFallback: p.NoContentFallback(),
			after:             p.After,
		}
		sendResult, err = p.Send(ctx, input)
	case string(FlavorOpencode):
		p := opencode.New()
		agentInfo = agentRuntime{
			name:              p.Name(),
			mode:              p.Mode(),
			stderrMode:        p.StderrMode(),
			emitExitError:     p.EmitExitError(),
			noContentFallback: p.NoContentFallback(),
			after:             p.After,
		}
		sendResult, err = p.Send(ctx, input)
	}
	if agentInfo.name == "" {
		cancel()
		m.mu.Unlock()
		return "", fmt.Errorf("unsupported agent: %s", proc.Req.Agent)
	}
	if err != nil {
		cancel()
		m.mu.Unlock()
		return "", fmt.Errorf("%s send failed: %w", agentInfo.name, err)
	}

	if sendResult.Stop {
		cancel()
		m.mu.Unlock()
		m.applyPipelineActions(sessionID, proc, agentInfo.name, sendResult.BeforeActions, false)
		reason := sendResult.StopReason
		if reason == "" {
			reason = "send-stopped"
		}
		m.emitRuntimeStateChanged(sessionID, reason)
		return "", nil
	}

	cmdOutput := sendResult.Command
	if cmdOutput.Cmd == nil {
		cancel()
		m.mu.Unlock()
		return "", fmt.Errorf("%s send returned empty command", agentInfo.name)
	}

	startedAt := time.Now().UnixMilli()
	if _, err := m.runtimes.Transition(sessionID, RuntimeTransitionSend, startedAt); err != nil {
		cancel()
		m.mu.Unlock()
		return "", err
	}
	proc.cancel = cancel
	m.mu.Unlock()

	if len(sendResult.BeforeActions) > 0 {
		m.applyPipelineActions(sessionID, proc, agentInfo.name, sendResult.BeforeActions, false)
	}

	requestID := uuid.New().String()

	// Always persist user prompts to database for all agent types
	raw := pipeline.BuildRoleWrappedMessage("user", map[string]interface{}{
		"type": "text",
		"text": sendResult.Input.Text,
	})
	m.emitMessage(sessionID, proc, raw, time.Now().UnixMilli())
	m.emitRuntimeStateChanged(sessionID, "started")

	go m.runOnce(ctx, cancel, sessionID, proc, agentInfo, sendResult)
	return requestID, nil
}

func (m *Manager) runOnce(ctx context.Context, cancel context.CancelFunc, sessionID string, proc *AgentProcess, agentInfo agentRuntime, sendResult pipeline.SendResult) {
	defer func() {
		cancel()
		m.mu.Lock()
		proc.cancel = nil
		m.mu.Unlock()
		_, _ = m.runtimes.Transition(sessionID, RuntimeTransitionComplete, 0)
		m.emitRuntimeStateChanged(sessionID, "completed")
	}()

	req := proc.Req
	cmdOutput := sendResult.Command
	if cmdOutput.AgentSessionID != "" && cmdOutput.AgentSessionID != proc.AgentSessionID {
		m.mu.Lock()
		proc.AgentSessionID = cmdOutput.AgentSessionID
		m.mu.Unlock()
		if m.onUpdateAgentSessId != nil {
			m.onUpdateAgentSessId(sessionID, cmdOutput.AgentSessionID, agentInfo.name)
		}
	}
	cmd := cmdOutput.Cmd
	if cmd == nil {
		return
	}
	cmd.Dir = req.Directory
	cmd.Env = append(filteredEnvWithoutClaudeCode(), "TERM=dumb")
	// 设置新进程组，abort 时可以杀整个进程树（包括孙进程）
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if agentInfo.mode == pipeline.ModeStream {
		m.runStreamCommand(ctx, cmd, sessionID, proc, agentInfo, sendResult.Input)
		return
	}

	// Detached pipelines: scanner handles output, command only triggers remote generation.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to start %s: %v", agentInfo.name, err)), time.Now().UnixMilli())
		return
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("%s exited with error: %v", agentInfo.name, err)), time.Now().UnixMilli())
	}
}

func (m *Manager) runStreamCommand(ctx context.Context, cmd *exec.Cmd, sessionID string, proc *AgentProcess, agentInfo agentRuntime, sendInput pipeline.SendInput) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to initialize %s output: %v", agentInfo.name, err)), time.Now().UnixMilli())
		return
	}

	var (
		stderrBuf strings.Builder
		stderrWG  sync.WaitGroup
	)
	if agentInfo.stderrMode == pipeline.StderrCapture {
		stderr, pipeErr := cmd.StderrPipe()
		if pipeErr != nil {
			m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to initialize %s error output: %v", agentInfo.name, pipeErr)), time.Now().UnixMilli())
			return
		}
		stderrWG.Add(1)
		go func() {
			defer stderrWG.Done()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				fmt.Fprintf(os.Stderr, "%s stderr: %s\n", agentInfo.name, line)
				stderrBuf.WriteString(line)
				stderrBuf.WriteString("\n")
			}
		}()
	} else {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("Failed to start %s: %v", agentInfo.name, err)), time.Now().UnixMilli())
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
		actions := agentInfo.after(sendInput, line, pipeline.ParseContext{
			NewID: func() string {
				return uuid.New().String()
			},
		})
		emitted = m.applyPipelineActions(sessionID, proc, agentInfo.name, actions, emitted)
	}

	waitErr := cmd.Wait()
	stderrWG.Wait()

	if ctx.Err() != nil {
		return
	}

	if stderrOutput := strings.TrimSpace(stderrBuf.String()); stderrOutput != "" {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("⚠️ %s Error:\n%s", titleCaseASCII(agentInfo.name), stderrOutput)), time.Now().UnixMilli())
	}
	if waitErr != nil && agentInfo.emitExitError {
		m.emitMessage(sessionID, proc, buildAssistantTextMessage(fmt.Sprintf("%s exited with error: %v", agentInfo.name, waitErr)), time.Now().UnixMilli())
		return
	}
	if !emitted {
		if fallback := agentInfo.noContentFallback; fallback != "" {
			m.emitMessage(sessionID, proc, buildAssistantTextMessage(fallback), time.Now().UnixMilli())
		}
	}
}

func (m *Manager) applyPipelineActions(sessionID string, proc *AgentProcess, agent string, actions []pipeline.Action, emitted bool) bool {
	handlers := map[pipeline.ActionKind]func(action pipeline.Action){
		pipeline.ActionBindSessionID: func(action pipeline.Action) {
			if !action.Force && (action.AgentSessionID == "" || proc.AgentSessionID == action.AgentSessionID) {
				return
			}
			m.mu.Lock()
			proc.AgentSessionID = action.AgentSessionID
			m.mu.Unlock()
			if m.onUpdateAgentSessId != nil {
				m.onUpdateAgentSessId(sessionID, action.AgentSessionID, agent)
			}
		},
		pipeline.ActionEmitMessage: func(action pipeline.Action) {
			if len(action.Message) == 0 {
				return
			}
			m.emitMessage(sessionID, proc, action.Message, time.Now().UnixMilli())
			emitted = true
		},
	}

	for _, action := range actions {
		handler, ok := handlers[action.Kind]
		if !ok {
			continue
		}
		handler(action)
	}
	return emitted
}

func filteredEnvWithoutClaudeCode() []string {
	env := make([]string, 0, 32)
	for _, e := range os.Environ() {
		if len(e) > 10 && e[:10] == "CLAUDECODE" {
			continue
		}
		env = append(env, e)
	}
	return env
}

func titleCaseASCII(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] = b[0] - ('a' - 'A')
	}
	return string(b)
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
			Type: SyncEventMessageAppended, SessionID: sessionID, Message: &msg,
		})
	}
}

func (m *Manager) emitRuntimeStateChanged(sessionID string, reason string) {
	if m.onEvent == nil {
		return
	}
	snap := m.runtimes.Get(sessionID)
	m.onEvent(sessionID, SyncEvent{
		Type:      SyncEventSessionStateChange,
		SessionID: sessionID,
		Data: SessionStateChangeData{
			State:     snap.State,
			RunningAt: snap.RunningAt,
			Reason:    reason,
		},
	})
}

func buildAssistantTextMessage(text string) json.RawMessage {
	return pipeline.BuildRoleWrappedMessage("assistant", text)
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

func (m *Manager) SetPermissionMode(sessionID string, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.agents[sessionID]
	if !ok {
		return
	}
	proc.Req.Yolo = IsYoloPermissionMode(proc.Req.Agent, mode)
}

func (m *Manager) SetModelMode(sessionID string, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc, ok := m.agents[sessionID]
	if !ok {
		return
	}
	proc.Req.Model = model
}

func toPipelineRequest(req CreateSessionRequest) pipeline.Request {
	return pipeline.Request{
		Agent:     req.Agent,
		Directory: req.Directory,
		Model:     req.Model,
		Yolo:      req.Yolo,
	}
}
