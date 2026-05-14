package act

import (
	"regexp"
	"strings"
)

var exprPattern = regexp.MustCompile(`\$\{\{\s*(.+?)\s*\}\}`)

// evaluateCondition evaluates a Harness expression condition string.
// Supports simple comparisons used in `if:` fields:
//   - env.VAR == "value"
//   - env.VAR != "value"
//   - compound && and || operators
//
// Returns true if the condition passes (step should run).
// Empty condition always returns true.
func evaluateCondition(expr string, env map[string]string, stepOutputs map[string]map[string]string) bool {
	if expr == "" {
		return true
	}

	raw := expr
	if m := exprPattern.FindStringSubmatch(expr); len(m) == 2 {
		raw = m[1]
	}

	return evalBoolExpr(raw, env, stepOutputs)
}

func evalBoolExpr(expr string, env map[string]string, outputs map[string]map[string]string) bool {
	// Handle || (OR) — lowest precedence
	if parts := splitOutsideQuotes(expr, "||"); len(parts) > 1 {
		for _, p := range parts {
			if evalBoolExpr(strings.TrimSpace(p), env, outputs) {
				return true
			}
		}
		return false
	}

	// Handle && (AND)
	if parts := splitOutsideQuotes(expr, "&&"); len(parts) > 1 {
		for _, p := range parts {
			if !evalBoolExpr(strings.TrimSpace(p), env, outputs) {
				return false
			}
		}
		return true
	}

	expr = strings.TrimSpace(expr)

	// Handle != comparison
	if parts := splitComparison(expr, "!="); len(parts) == 2 {
		left := resolveValue(strings.TrimSpace(parts[0]), env, outputs)
		right := resolveValue(strings.TrimSpace(parts[1]), env, outputs)
		return left != right
	}

	// Handle == comparison
	if parts := splitComparison(expr, "=="); len(parts) == 2 {
		left := resolveValue(strings.TrimSpace(parts[0]), env, outputs)
		right := resolveValue(strings.TrimSpace(parts[1]), env, outputs)
		return left == right
	}

	// Bare truthy check
	val := resolveValue(expr, env, outputs)
	return val != "" && val != "false" && val != "0"
}

func resolveValue(s string, env map[string]string, outputs map[string]map[string]string) string {
	// String literal
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}

	// env.VAR
	if strings.HasPrefix(s, "env.") {
		key := s[4:]
		return env[key]
	}

	// steps.STEP_ID.outputs.VAR
	if strings.HasPrefix(s, "steps.") {
		parts := strings.SplitN(s, ".", 4)
		if len(parts) == 4 && parts[2] == "outputs" {
			stepID := parts[1]
			varName := parts[3]
			if outs, ok := outputs[stepID]; ok {
				return outs[varName]
			}
		}
		return ""
	}

	return s
}

// expandExpressions replaces ${{ ... }} in environment variable values
// with resolved values from env and step outputs.
func expandExpressions(env map[string]string, stepOutputs map[string]map[string]string) map[string]string {
	result := make(map[string]string, len(env))
	for k, v := range env {
		result[k] = exprPattern.ReplaceAllStringFunc(v, func(match string) string {
			m := exprPattern.FindStringSubmatch(match)
			if len(m) != 2 {
				return match
			}
			return resolveValue(strings.TrimSpace(m[1]), env, stepOutputs)
		})
	}
	return result
}

func splitOutsideQuotes(s, sep string) []string {
	var parts []string
	depth := 0
	inQuote := byte(0)
	start := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		}
		if depth == 0 && i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	parts = append(parts, s[start:])

	if len(parts) == 1 {
		return nil
	}
	return parts
}

func splitComparison(s, op string) []string {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			continue
		}
		if i+len(op) <= len(s) && s[i:i+len(op)] == op {
			return []string{s[:i], s[i+len(op):]}
		}
	}
	return nil
}
