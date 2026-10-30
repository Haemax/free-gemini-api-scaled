package toolcall

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ToolCallFormat specifies the expected tool call format
type ToolCallFormat string

const (
	FormatXML  ToolCallFormat = "xml"  // <function=name>...
	FormatJSON ToolCallFormat = "json" // {"name":...}
)

// RawToolCall represents an extracted tool call
type RawToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

var (
	// Matches ... or ```json ... ``` containing a tool call object/array
	jsonBlockRegex = regexp.MustCompile(`(?s)"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{.*?\})`)
	xmlTagRegex    = regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_-]+)>(.*?)</function>`)
)

func generateID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "call_fallback"
	}
	return "call_" + hex.EncodeToString(b)
}

// extractJSONArguments parses JSON tool call argument objects safely by tracking bracket depth
func extractJSONArguments(text string, startIdx int) (map[string]any, int) {
	if startIdx >= len(text) || text[startIdx] != '{' {
		return nil, -1
	}
	depth := 0
	inString := false
	escaped := false

	for i := startIdx; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if !inString {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					argStr := text[startIdx : i+1]
					var args map[string]any
					if err := json.Unmarshal([]byte(argStr), &args); err == nil {
						return args, i + 1
					}
					return nil, -1
				}
			}
		}
	}
	return nil, -1
}

// NormalizeToolCalls detects and cleans up tool call syntax across model output variants
func NormalizeToolCalls(rawText string) (string, []RawToolCall) {
	var calls []RawToolCall
	cleaned := rawText

	// 1. Structural scanner for nested JSON objects
	searchIdx := 0
	for searchIdx < len(rawText) {
		nameLoc := strings.Index(rawText[searchIdx:], `"name"`)
		if nameLoc == -1 {
			break
		}
		absNameIdx := searchIdx + nameLoc

		argsKeyLoc := strings.Index(rawText[absNameIdx:], `"arguments"`)
		if argsKeyLoc != -1 {
			absArgsKeyIdx := absNameIdx + argsKeyLoc
			braceIdx := strings.IndexByte(rawText[absArgsKeyIdx:], '{')
			if braceIdx != -1 {
				absBraceIdx := absArgsKeyIdx + braceIdx
				if args, nextIdx := extractJSONArguments(rawText, absBraceIdx); nextIdx != -1 {
					nameSub := rawText[absNameIdx:absArgsKeyIdx]
					if m := regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`).FindStringSubmatch(nameSub); len(m) > 1 {
						calls = append(calls, RawToolCall{
							ID:        generateID(),
							Name:      m[1],
							Arguments: args,
						})
						searchIdx = nextIdx
						continue
					}
				}
			}
		}
		searchIdx = absNameIdx + 6
	}

	// Fallback regex pattern matching
	if len(calls) == 0 {
		matches := jsonBlockRegex.FindAllStringSubmatch(rawText, -1)
		for _, m := range matches {
			if len(m) >= 3 {
				name := m[1]
				var args map[string]any
				if err := json.Unmarshal([]byte(m[2]), &args); err == nil {
					calls = append(calls, RawToolCall{
						ID:        generateID(),
						Name:      name,
						Arguments: args,
					})
				}
			}
		}
	}

	// Extract XML pattern tool calls
	xmlMatches := xmlTagRegex.FindAllStringSubmatch(rawText, -1)
	for _, m := range xmlMatches {
		if len(m) >= 3 {
			name := m[1]
			var args map[string]any
			if err := json.Unmarshal([]byte(m[2]), &args); err == nil {
				calls = append(calls, RawToolCall{
					ID:        generateID(),
					Name:      name,
					Arguments: args,
				})
			}
		}
	}

	return cleaned, calls
}

// FormatToolCalls converts normalized calls back to standard JSON output format
func FormatToolCalls(calls []RawToolCall) (string, error) {
	if len(calls) == 0 {
		return "", errors.New("no calls to format")
	}

	var sb strings.Builder
	for _, tc := range calls {
		argBytes, err := json.Marshal(tc.Arguments)
		if err != nil {
			continue
		}
		sb.WriteString("\n")
		if tc.ID != "" {
			sb.WriteString(fmt.Sprintf(`{"id": "%s", "name": "%s", "arguments": `, tc.ID, tc.Name))
		} else {
			sb.WriteString(fmt.Sprintf(`{"name": "%s", "arguments": `, tc.Name))
		}
		sb.Write(argBytes)
		sb.WriteString("}\n\n")
	}
	return sb.String(), nil
}

