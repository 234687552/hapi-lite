package session

type AgentSessionIDAccessor struct {
	Get func(meta *Metadata) string
	Set func(meta *Metadata, value string)
}

var agentSessionIDAccessors = map[string]AgentSessionIDAccessor{
	string(FlavorClaude): {
		Get: func(meta *Metadata) string { return meta.ClaudeSessionID },
		Set: func(meta *Metadata, value string) { meta.ClaudeSessionID = value },
	},
	string(FlavorCodex): {
		Get: func(meta *Metadata) string { return meta.CodexSessionID },
		Set: func(meta *Metadata, value string) { meta.CodexSessionID = value },
	},
	string(FlavorGemini): {
		Get: func(meta *Metadata) string { return meta.GeminiSessionID },
		Set: func(meta *Metadata, value string) { meta.GeminiSessionID = value },
	},
	string(FlavorOpencode): {
		Get: func(meta *Metadata) string { return meta.OpencodeSessionID },
		Set: func(meta *Metadata, value string) { meta.OpencodeSessionID = value },
	},
}

func GetAgentSessionID(meta *Metadata, agent string) string {
	if meta == nil {
		return ""
	}
	accessor, ok := agentSessionIDAccessors[agent]
	if !ok || accessor.Get == nil {
		return ""
	}
	return accessor.Get(meta)
}

func SetAgentSessionID(meta *Metadata, agent, value string) bool {
	if meta == nil {
		return false
	}
	accessor, ok := agentSessionIDAccessors[agent]
	if !ok || accessor.Set == nil {
		return false
	}
	accessor.Set(meta, value)
	return true
}

type SlashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

var defaultSlashCommands = []SlashCommand{
	{Name: "clear", Description: "Clear conversation history", Source: "builtin"},
	{Name: "status", Description: "Show session status", Source: "builtin"},
}

var slashCommandsByAgent = map[string][]SlashCommand{
	string(FlavorClaude): defaultSlashCommands,
	string(FlavorCodex): {
		{Name: "review", Description: "Review current changes and find issues", Source: "builtin"},
		{Name: "status", Description: "Show current session status", Source: "builtin"},
	},
	string(FlavorGemini): {
		{Name: "about", Description: "Show version info", Source: "builtin"},
		{Name: "clear", Description: "Clear the conversation", Source: "builtin"},
	},
	string(FlavorOpencode): {},
}

var yoloPermissionModesByAgent = map[string]map[string]struct{}{
	string(FlavorClaude): {
		"bypassPermissions": {},
	},
	string(FlavorCodex): {
		"yolo": {},
	},
	string(FlavorGemini): {
		"yolo": {},
	},
	string(FlavorOpencode): {
		"yolo": {},
	},
}

func IsYoloPermissionMode(agent, mode string) bool {
	allowedModes, ok := yoloPermissionModesByAgent[agent]
	if !ok {
		allowedModes = yoloPermissionModesByAgent[string(FlavorClaude)]
	}
	_, exists := allowedModes[mode]
	return exists
}

func SlashCommandsForAgent(agent string) []SlashCommand {
	commands, ok := slashCommandsByAgent[agent]
	if !ok {
		commands = defaultSlashCommands
	}
	out := make([]SlashCommand, len(commands))
	copy(out, commands)
	return out
}
