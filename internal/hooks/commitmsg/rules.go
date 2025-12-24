package commitmsg

import (
	"regexp"
)

// RuleViolation represents a failed rule check.
type RuleViolation struct {
	Rule    Rule
	Matched bool // For deny rules: true means pattern matched (violation)
	// For require rules: false means pattern didn't match (violation)
}

// EvaluateRules evaluates all rules against a parsed commit message.
// Returns a slice of violations (empty if all rules pass).
func EvaluateRules(rules []Rule, message ParsedCommitMessage) []RuleViolation {
	var violations []RuleViolation

	for _, rule := range rules {
		// Get the text to check based on scope
		text := getTextForScope(rule.Scope, message)

		// Use cached regex
		matched := rule.regex.MatchString(text)

		// Check if rule is violated
		violated := false
		if rule.Type == RuleTypeDeny && matched {
			violated = true
		}

		if rule.Type == RuleTypeRequire && !matched {
			violated = true
		}

		if violated {
			violations = append(violations, RuleViolation{
				Rule:    rule,
				Matched: matched,
			})
		}
	}

	return violations
}

// shouldSkipAuthor checks if a commit author should be skipped based on patterns.
func shouldSkipAuthor(name string, email string, patterns []string) bool {
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			// Invalid pattern, skip it
			continue
		}

		// Check if pattern matches either name or email
		if re.MatchString(name) || re.MatchString(email) {
			return true
		}
	}

	return false
}

func getTextForScope(scope Scope, message ParsedCommitMessage) string {
	switch scope {
	case ScopeTitle:
		return message.Title

	case ScopeBody:
		return message.Body

	case ScopeFooter:
		return message.Footer

	case ScopeMessage:
		return message.Raw

	default:
		return ""
	}
}
