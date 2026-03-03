package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Mode string

const (
	ModeStream   Mode = "stream"
	ModeDetached Mode = "detached"
)

type StderrMode string

const (
	StderrCapture     StderrMode = "capture"
	StderrPassthrough StderrMode = "passthrough"
)

type ActionKind string

const (
	ActionEmitMessage   ActionKind = "emit_message"
	ActionBindSessionID ActionKind = "bind_session_id"
)

type Request struct {
	Agent     string
	Directory string
	Model     string
	Yolo      bool
}

type SendInput struct {
	Request        Request
	Text           string
	AgentSessionID string
}

type BeforeResult struct {
	Input      SendInput
	Actions    []Action
	Stop       bool
	StopReason string
}

type CommandOutput struct {
	Cmd            *exec.Cmd
	AgentSessionID string
}

type SendHandler func(ctx context.Context, input SendInput) (CommandOutput, error)

type SendResult struct {
	Input         SendInput
	BeforeActions []Action
	Stop          bool
	StopReason    string
	Command       CommandOutput
}

type ParseContext struct {
	NewID func() string
}

type Action struct {
	Kind           ActionKind
	Message        json.RawMessage
	AgentSessionID string
	Force          bool
}

type Pipeline interface {
	Name() string
	Mode() Mode
	StderrMode() StderrMode
	EmitExitError() bool
	NoContentFallback() string
	Send(ctx context.Context, input SendInput) (SendResult, error)
	After(input SendInput, line []byte, ctx ParseContext) []Action
}

func ValidateSendInput(input SendInput) error {
	if strings.TrimSpace(input.Text) == "" {
		return fmt.Errorf("empty send input")
	}
	return nil
}

func ResolveBefore(input SendInput, beforeMap map[string]func(SendInput) BeforeResult) (SendResult, bool) {
	trimmed := strings.TrimSpace(input.Text)
	handler, ok := beforeMap[trimmed]
	if !ok || handler == nil {
		return SendResult{}, false
	}
	result := handler(input)
	return SendResult{
		Input:         result.Input,
		BeforeActions: result.Actions,
		Stop:          result.Stop,
		StopReason:    result.StopReason,
	}, true
}

func BuildSendResult(ctx context.Context, input SendInput, key string, handlers map[string]SendHandler) (SendResult, error) {
	handler, ok := handlers[key]
	if !ok || handler == nil {
		return SendResult{}, fmt.Errorf("unknown send key: %s", key)
	}
	return BuildSingleSendResult(ctx, input, handler)
}

func BuildSingleSendResult(ctx context.Context, input SendInput, handler SendHandler) (SendResult, error) {
	if handler == nil {
		return SendResult{}, fmt.Errorf("missing send handler")
	}
	cmdOutput, err := handler(ctx, input)
	if err != nil {
		return SendResult{}, err
	}
	return SendResult{
		Input:   input,
		Command: cmdOutput,
	}, nil
}

func BuildRoleWrappedMessage(role string, content interface{}) json.RawMessage {
	b, _ := json.Marshal(map[string]interface{}{
		"role":    role,
		"content": content,
	})
	return json.RawMessage(b)
}
