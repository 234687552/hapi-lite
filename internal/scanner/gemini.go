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
}

func NewGeminiScanner(onMsg MessageCallback) *GeminiScanner {
	return &GeminiScanner{
		stopCh:    make(chan struct{}),
		onMessage: onMsg,
	}
}

func (s *GeminiScanner) Start(sessionID, agentSessionID, dir string) error {
	s.sessionID = sessionID
	go s.watch()
	return nil
}

func (s *GeminiScanner) Stop() { close(s.stopCh) }

func (s *GeminiScanner) watch() {
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

	tailJSONL(s.stopCh, path, s.sessionID, "gemini", s.onMessage)
}
