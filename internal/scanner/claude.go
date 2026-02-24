package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// ClaudeScanner monitors ~/.claude/projects/ JSONL files
type ClaudeScanner struct {
	sessionID string
	stopCh    chan struct{}
	onMessage MessageCallback
	onState   StateCallback
}

func NewClaudeScanner(onMsg MessageCallback, onState StateCallback) *ClaudeScanner {
	return &ClaudeScanner{
		stopCh:    make(chan struct{}),
		onMessage: onMsg,
		onState:   onState,
	}
}

func (s *ClaudeScanner) Start(sessionID, agentSessionID, dir string) error {
	s.sessionID = sessionID
	go s.watch(agentSessionID, dir)
	return nil
}

func (s *ClaudeScanner) Stop() { close(s.stopCh) }

func (s *ClaudeScanner) watch(agentSessionID, dir string) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".claude", "projects")

	// Compute project-specific directory like Claude does
	projectDir := ""
	if dir != "" {
		absDir, _ := filepath.Abs(dir)
		projectID := nonAlphaNum.ReplaceAllString(absDir, "-")
		projectDir = filepath.Join(baseDir, projectID)
	}

	var jsonlPath string
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		if agentSessionID != "" {
			matches, _ := filepath.Glob(filepath.Join(baseDir, "*", agentSessionID+".jsonl"))
			if len(matches) > 0 {
				jsonlPath = matches[0]
				break
			}
		}

		// Try project-specific directory first
		if projectDir != "" {
			jsonlPath = findNewestFile(projectDir, ".jsonl")
			if jsonlPath != "" {
				break
			}
		}

		// Fallback to all projects
		jsonlPath = findNewestFile(baseDir, ".jsonl")
		if jsonlPath != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	tailJSONL(s.stopCh, jsonlPath, s.sessionID, s.onMessage)
}
