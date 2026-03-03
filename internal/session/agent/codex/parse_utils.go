package codex

import (
	"encoding/json"
	"strings"
)

var codexItemTypeAliases = map[string]string{
	"tool_call":            "tool_call",
	"tool-call":            "tool_call",
	"function_call":        "tool_call",
	"tool_result":          "tool_result",
	"tool-call-result":     "tool_result",
	"function_call_output": "tool_result",
}

func normalizeCodexItemType(itemType string) string {
	if normalized, ok := codexItemTypeAliases[itemType]; ok {
		return normalized
	}
	return itemType
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func firstString(values ...interface{}) string {
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func parseMaybeJSONValue(v interface{}) interface{} {
	s, ok := v.(string)
	if !ok {
		return v
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed interface{}
		if json.Unmarshal([]byte(trimmed), &parsed) == nil {
			return parsed
		}
	}
	return s
}
