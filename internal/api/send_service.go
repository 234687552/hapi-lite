package api

import (
	"errors"
	"fmt"
	"strings"

	"github.com/liangzd/hapi-lite/internal/session"
	"github.com/liangzd/hapi-lite/internal/sse"
	"github.com/liangzd/hapi-lite/internal/store"
)

var ErrSessionNotFound = errors.New("session not found")

type SendRuntime struct {
	Active     bool                 `json:"active"`
	Thinking   bool                 `json:"thinking"`
	ThinkingAt int64                `json:"thinkingAt"`
	State      session.RuntimeState `json:"state"`
}

type SendResult struct {
	OK        bool        `json:"ok"`
	RequestID string      `json:"requestId,omitempty"`
	Runtime   SendRuntime `json:"runtime"`
}

type SendService struct {
	Store  *store.Store
	Broker *sse.Broker
	Mgr    *session.Manager
}

func NewSendService(st *store.Store, broker *sse.Broker, mgr *session.Manager) *SendService {
	return &SendService{
		Store:  st,
		Broker: broker,
		Mgr:    mgr,
	}
}

func (s *SendService) Send(sessionID string, req session.SendMessageRequest) (*SendResult, error) {
	if s.Mgr == nil {
		return nil, errors.New("manager unavailable")
	}
	if s.Store == nil {
		return nil, errors.New("store unavailable")
	}

	text := composeSendText(req.Text, req.Attachments)
	if err := s.ensureReadySession(sessionID); err != nil {
		return nil, err
	}

	requestID, err := s.Mgr.SendMessage(sessionID, text)
	if err != nil {
		return nil, err
	}

	snap := s.Mgr.RuntimeSnapshot(sessionID)
	return &SendResult{
		OK:        true,
		RequestID: requestID,
		Runtime: SendRuntime{
			Active:     s.Mgr.HasAgent(sessionID),
			Thinking:   snap.State == session.RuntimeStateRunning,
			ThinkingAt: snap.RunningAt,
			State:      snap.State,
		},
	}, nil
}

func (s *SendService) ensureReadySession(sessionID string) error {
	if s.Mgr.HasAgent(sessionID) {
		return nil
	}

	sess, err := s.Store.GetSession(sessionID)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	dir := "."
	flavor := string(session.FlavorClaude)
	if sess.Metadata != nil {
		if sess.Metadata.Path != "" {
			dir = sess.Metadata.Path
		}
		if sess.Metadata.Flavor != "" {
			flavor = sess.Metadata.Flavor
		}
	}

	startSeq, countErr := s.Store.GetMessageCount(sessionID)
	if countErr != nil {
		return countErr
	}
	agentSID := session.GetAgentSessionID(sess.Metadata, flavor)

	s.Mgr.SpawnAgentWithSession(sessionID, session.CreateSessionRequest{
		Directory: dir,
		Agent:     flavor,
		Model:     sess.ModelMode,
	}, startSeq, agentSID)
	_ = s.Store.SetSessionActive(sessionID, true)
	s.publishRuntimeStateChange(sessionID, "resumed")
	return nil
}

func composeSendText(text string, attachments []session.AttachmentMetadata) string {
	if len(attachments) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\nAttached files:\n")
	for _, att := range attachments {
		if att.Path == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("- %s (%s)\n", att.Filename, att.Path))
	}
	return b.String()
}

func (s *SendService) publishRuntimeStateChange(sessionID string, reason string) {
	if s.Broker == nil || s.Mgr == nil {
		return
	}
	snap := s.Mgr.RuntimeSnapshot(sessionID)
	s.Broker.Publish(session.SyncEvent{
		Type:      session.SyncEventSessionStateChange,
		SessionID: sessionID,
		Data: session.SessionStateChangeData{
			State:     snap.State,
			RunningAt: snap.RunningAt,
			Reason:    reason,
		},
	})
}
