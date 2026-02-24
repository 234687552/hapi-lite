package scanner

import (
	"os"
	"path/filepath"
	"time"
)

type OpencodeScanner struct {
	sessionID string
	stopCh    chan struct{}
	onMessage MessageCallback
	onState   StateCallback
}

func NewOpencodeScanner(onMsg MessageCallback, onState StateCallback) *OpencodeScanner {
	return &OpencodeScanner{
		stopCh:    make(chan struct{}),
		onMessage: onMsg,
		onState:   onState,
	}
}

func (s *OpencodeScanner) Start(sessionID, agentSessionID, dir string) error {
	s.sessionID = sessionID
	go s.watch()
	return nil
}

func (s *OpencodeScanner) Stop() { close(s.stopCh) }

func (s *OpencodeScanner) watch() {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".opencode", "sessions")

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
