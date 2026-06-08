package redact

import (
	"regexp"
	"strings"
)

const Mask = "[redacted]"

var textPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{
		pattern:     regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
		replacement: Mask,
	},
	{
		pattern:     regexp.MustCompile(`(?i)((?:"?(?:password|passwd|private[_-]?key|privatekey|secret|token|authorization|cookie|credential)"?)\s*[:=]\s*)(Bearer\s+[^\s,;]+|"[^"]*"|'[^']*'|[^\s,;]+)`),
		replacement: `${1}"` + Mask + `"`,
	},
}

func Metadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return redactMap(metadata)
}

func Text(input string) string {
	output := input
	for _, redactor := range textPatterns {
		output = redactor.pattern.ReplaceAllString(output, redactor.replacement)
	}
	return output
}

func Any(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed)
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, Any(item))
		}
		return values
	default:
		return value
	}
}

func redactMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		if IsSensitiveKey(key) {
			output[key] = Mask
			continue
		}
		output[key] = Any(value)
	}
	return output
}

func IsSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, token := range []string{
		"password",
		"passwd",
		"private_key",
		"privatekey",
		"secret",
		"token",
		"authorization",
		"cookie",
		"credential",
	} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}
