package template

import (
	"regexp"
	"strings"
)

var variableRegex = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Render substitutes variables in the template content.
func Render(content string, variables map[string]string) string {
	result := variableRegex.ReplaceAllStringFunc(content, func(match string) string {
		key := strings.Trim(match, "{}")
		if val, ok := variables[key]; ok {
			return val
		}
		return match // Leave unmatched variables as-is
	})
	return result
}

// ExtractVariables extracts all variable names from template content.
func ExtractVariables(content string) []string {
	matches := variableRegex.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var vars []string

	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			vars = append(vars, m[1])
			seen[m[1]] = true
		}
	}
	return vars
}
