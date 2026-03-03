package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// findNewestFile returns the most recently modified file with the given extension in dir.
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

// tailJSONL tails a JSONL file from the current end, emitting mapped messages.
func tailJSONL(stopCh chan struct{}, path, sessionID, agent string, onMsg MessageCallback) {
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
			var peek struct {
				Type string `json:"type"`
			}
			json.Unmarshal(line, &peek)
			if !shouldEmitRawEnvelope(agent, peek.Type) {
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

func shouldEmitRawEnvelope(agent, eventType string) bool {
	switch agent {
	case "claude", "gemini", "opencode":
		return eventType == "user" || eventType == "assistant" || eventType == "summary"
	default:
		return false
	}
}
