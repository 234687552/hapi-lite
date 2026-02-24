package scanner

import (
	"os"
	"path/filepath"
	"time"
)

type GeminiScanner struct {
	sessionID string
	stopCh    chan struct{}
	onMessage MessageCallback
	onState   StateCallback
}

func NewGeminiScanner(onMsg MessageCallback, onState StateCallback) *GeminiScanner {
	return &GeminiScanner{
		stopCh:    make(chan struct{}),
		onMessage: onMsg,
		onState:   onState,
	}
}

func (s *GeminiScanner) Start(sessionID, agentSessionID, dir string) error {
	s.sessionID = sessionID
	go s.watch(agentSessionID)
	return nil
}

func (s *GeminiScanner) Stop() { close(s.stopCh) }

func (s *GeminiScanner) watch(agentSessionID string) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".gemini", "sessions")

	var path string
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		path = findNewestFile(baseDir, ".jsonl")
		if path != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	tailJSONL(s.stopCh, path, s.sessionID, s.onMessage)
}
