package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CodexScanner struct {
	sessionID string
	stopCh    chan struct{}
	onMessage MessageCallback
	onState   StateCallback
	metaCache map[string]codexFileMeta
}

func NewCodexScanner(onMsg MessageCallback, onState StateCallback) *CodexScanner {
	return &CodexScanner{
		stopCh:    make(chan struct{}),
		onMessage: onMsg,
		onState:   onState,
		metaCache: map[string]codexFileMeta{},
	}
}

func (s *CodexScanner) Start(sessionID, agentSessionID, dir string) error {
	s.sessionID = sessionID
	go s.watch(agentSessionID, dir)
	return nil
}

func (s *CodexScanner) Stop() { close(s.stopCh) }

type codexFileMeta struct {
	modTime int64
	cwd     string
}

func (s *CodexScanner) watch(agentSessionID, dir string) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".codex", "sessions")
	targetCwd := normalizeScannerPath(dir)

	var currentPath string
	offsets := map[string]int64{}
	bootBound := false
	var seq int64
	lastLookupAt := time.Time{}

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		if currentPath == "" || time.Since(lastLookupAt) >= time.Second {
			path := s.pickSessionFile(baseDir, agentSessionID, targetCwd)
			lastLookupAt = time.Now()

			if path != "" && path != currentPath {
				currentPath = path
				if _, ok := offsets[path]; !ok {
					if !bootBound {
						if info, err := os.Stat(path); err == nil {
							offsets[path] = info.Size()
						} else {
							offsets[path] = 0
						}
						bootBound = true
					} else {
						offsets[path] = 0
					}
				}
			}
		}

		if currentPath == "" {
			time.Sleep(300 * time.Millisecond)
			continue
		}

		nextOffset, raws := readCodexMessages(currentPath, offsets[currentPath])
		offsets[currentPath] = nextOffset
		for _, raw := range raws {
			seq++
			if s.onMessage != nil {
				s.onMessage(s.sessionID, ScannedMessage{
					ID:        uuid.New().String(),
					SessionID: s.sessionID,
					Seq:       seq,
					Content:   raw,
					CreatedAt: time.Now().UnixMilli(),
				})
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *CodexScanner) pickSessionFile(baseDir, agentSessionID, targetCwd string) string {
	var newest string
	var newestTime time.Time
	sessionSuffix := "-" + agentSessionID + ".jsonl"

	filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}

		base := filepath.Base(path)
		if agentSessionID != "" {
			if base != agentSessionID+".jsonl" && !strings.HasSuffix(base, sessionSuffix) {
				return nil
			}
		} else if targetCwd != "" {
			cwd := s.readSessionCwd(path, info)
			if cwd == "" || !sameScannerPath(cwd, targetCwd) {
				return nil
			}
		}

		if info.ModTime().After(newestTime) {
			newest = path
			newestTime = info.ModTime()
		}
		return nil
	})

	if newest != "" {
		return newest
	}

	// Avoid consuming unrelated Codex sessions when we know the target cwd.
	if targetCwd != "" && agentSessionID == "" {
		return ""
	}
	return findNewestFile(baseDir, ".jsonl")
}

func (s *CodexScanner) readSessionCwd(path string, info os.FileInfo) string {
	mod := info.ModTime().UnixNano()
	if cached, ok := s.metaCache[path]; ok && cached.modTime == mod {
		return cached.cwd
	}
	cwd := readCodexSessionCwd(path)
	s.metaCache[path] = codexFileMeta{modTime: mod, cwd: cwd}
	return cwd
}

func readCodexSessionCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2*1024*1024)

	for i := 0; sc.Scan() && i < 120; i++ {
		line := sc.Bytes()
		var record map[string]interface{}
		if json.Unmarshal(line, &record) != nil {
			continue
		}
		if asString(record["type"]) != "session_meta" {
			continue
		}
		payload, _ := record["payload"].(map[string]interface{})
		cwd := asString(payload["cwd"])
		if cwd != "" {
			return cwd
		}
	}
	return ""
}

func readCodexMessages(path string, offset int64) (int64, []json.RawMessage) {
	f, err := os.Open(path)
	if err != nil {
		return offset, nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return offset, nil
	}
	if info.Size() < offset {
		offset = 0
	}
	if info.Size() <= offset {
		return info.Size(), nil
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return offset, nil
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 128*1024), 8*1024*1024)

	var out []json.RawMessage
	for sc.Scan() {
		for _, raw := range convertCodexLineToMessages(sc.Bytes()) {
			out = append(out, raw)
		}
	}
	return info.Size(), out
}

func convertCodexLineToMessages(line []byte) []json.RawMessage {
	var record map[string]interface{}
	if json.Unmarshal(line, &record) != nil {
		return nil
	}

	topType := asString(record["type"])
	payload, _ := record["payload"].(map[string]interface{})
	if payload == nil {
		return nil
	}

	switch topType {
	case "event_msg":
		return convertCodexEvent(payload)
	case "response_item":
		return convertCodexResponseItem(payload)
	default:
		return nil
	}
}

func convertCodexEvent(payload map[string]interface{}) []json.RawMessage {
	eventType := asString(payload["type"])
	switch eventType {
	case "user_message":
		text := sanitizeText(firstString(payload["message"], payload["text"], payload["content"]))
		if text == "" {
			return nil
		}
		return []json.RawMessage{buildUserTextMessage(text)}
	case "agent_message":
		text := sanitizeText(firstString(payload["message"], payload["text"]))
		if text == "" {
			return nil
		}
		return []json.RawMessage{buildCodexAgentMessage(map[string]interface{}{
			"type":    "message",
			"message": text,
			"id":      uuid.New().String(),
		})}
	case "agent_reasoning":
		text := sanitizeText(firstString(payload["text"], payload["message"]))
		if text == "" {
			return nil
		}
		return []json.RawMessage{buildCodexAgentMessage(map[string]interface{}{
			"type":    "reasoning",
			"message": text,
			"id":      uuid.New().String(),
		})}
	default:
		return nil
	}
}

func convertCodexResponseItem(payload map[string]interface{}) []json.RawMessage {
	itemType := asString(payload["type"])
	switch itemType {
	case "function_call":
		name := asString(payload["name"])
		callID := extractCallID(payload)
		if name == "" || callID == "" {
			return nil
		}
		return []json.RawMessage{buildCodexAgentMessage(map[string]interface{}{
			"type":   "tool-call",
			"name":   name,
			"callId": callID,
			"input":  parseMaybeJSON(payload["arguments"]),
			"id":     uuid.New().String(),
		})}
	case "function_call_output":
		callID := extractCallID(payload)
		if callID == "" {
			return nil
		}
		return []json.RawMessage{buildCodexAgentMessage(map[string]interface{}{
			"type":   "tool-call-result",
			"callId": callID,
			"output": payload["output"],
			"id":     uuid.New().String(),
		})}
	default:
		return nil
	}
}

func buildUserTextMessage(text string) json.RawMessage {
	return buildRoleWrappedMessage("user", map[string]interface{}{
		"type": "text",
		"text": text,
	})
}

func buildCodexAgentMessage(data map[string]interface{}) json.RawMessage {
	return buildRoleWrappedMessage("agent", map[string]interface{}{
		"type": "codex",
		"data": data,
	})
}

func buildRoleWrappedMessage(role string, content interface{}) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"role":    role,
		"content": content,
	})
	return json.RawMessage(b)
}

func extractCallID(payload map[string]interface{}) string {
	return firstString(
		payload["call_id"],
		payload["callId"],
		payload["tool_call_id"],
		payload["toolCallId"],
		payload["id"],
	)
}

func parseMaybeJSON(v interface{}) interface{} {
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

func sanitizeText(v string) string {
	return strings.TrimRight(v, "\r\n")
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func firstString(values ...interface{}) string {
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func normalizeScannerPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func sameScannerPath(a, b string) bool {
	return normalizeScannerPath(a) == normalizeScannerPath(b)
}

// Shared helpers for legacy JSONL scanners (claude/gemini/opencode fallback).
func findNewestFile(dir, ext string) string {
	var newest string
	var newestTime time.Time
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ext && info.ModTime().After(newestTime) {
			newest = path
			newestTime = info.ModTime()
		}
		return nil
	})
	return newest
}

func tailJSONL(stopCh chan struct{}, path, sessionID string, onMsg MessageCallback) {
	// Start from current file end — only capture new messages
	var offset int64
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	}
	var seq int64

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		f, err := os.Open(path)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		info, _ := f.Stat()
		if info.Size() <= offset {
			f.Close()
			time.Sleep(200 * time.Millisecond)
			continue
		}

		f.Seek(offset, 0)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)

		for sc.Scan() {
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var raw json.RawMessage
			if json.Unmarshal(line, &raw) != nil {
				continue
			}
			// Only allow conversation messages through
			var peek struct {
				Type string `json:"type"`
			}
			json.Unmarshal(line, &peek)
			if peek.Type != "user" && peek.Type != "assistant" && peek.Type != "summary" {
				continue
			}
			seq++
			if onMsg != nil {
				onMsg(sessionID, ScannedMessage{
					ID:        uuid.New().String(),
					SessionID: sessionID,
					Seq:       seq,
					Content:   raw,
					CreatedAt: time.Now().UnixMilli(),
				})
			}
		}

		offset = info.Size()
		f.Close()
		time.Sleep(200 * time.Millisecond)
	}
}
