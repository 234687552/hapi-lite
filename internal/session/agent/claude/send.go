package claude

import (
	"context"
	"os/exec"

	"github.com/google/uuid"
	pipeline "github.com/liangzd/hapi-lite/internal/session/agent"
)

func buildResumeCommand(ctx context.Context, input pipeline.SendInput) (pipeline.CommandOutput, error) {
	args := baseArgs(input.Request)
	args = append(args, "--resume", input.AgentSessionID, input.Text)
	return pipeline.CommandOutput{
		Cmd:            exec.CommandContext(ctx, "claude", args...),
		AgentSessionID: input.AgentSessionID,
	}, nil
}

func buildNewCommand(ctx context.Context, input pipeline.SendInput) (pipeline.CommandOutput, error) {
	sessionID := uuid.New().String()
	args := baseArgs(input.Request)
	args = append(args, "--session-id", sessionID, input.Text)
	return pipeline.CommandOutput{
		Cmd:            exec.CommandContext(ctx, "claude", args...),
		AgentSessionID: sessionID,
	}, nil
}

func baseArgs(req pipeline.Request) []string {
	args := []string{"--print", "--verbose", "--output-format", "stream-json"}
	if req.Yolo {
		args = append(args, "--dangerously-skip-permissions")
	}
	if req.Model != "" && req.Model != "default" {
		args = append(args, "--model", req.Model)
	}
	return args
}
