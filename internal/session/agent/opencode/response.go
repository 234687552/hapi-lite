package opencode

import pipeline "github.com/liangzd/hapi-lite/internal/session/agent"

func (p *Pipeline) After(_ pipeline.SendInput, _ []byte, _ pipeline.ParseContext) []pipeline.Action {
	return nil
}
