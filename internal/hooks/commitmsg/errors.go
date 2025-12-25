package commitmsg

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// formatViolationError creates a detailed error message for rule violations.
func formatViolationError(commit *object.Commit, ref string, violations []RuleViolation) error {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Commit %s in %s failed validation:\n", commit.Hash.String()[:7], ref))
	sb.WriteString(fmt.Sprintf("Commit message: %s\n\n", getFirstLine(commit.Message)))

	sb.WriteString("Rule violations:\n")
	for i, v := range violations {
		sb.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, v.Rule.Name, getViolationMessage(v)))

		if v.Rule.Type == RuleTypeDeny {
			sb.WriteString(fmt.Sprintf("     Pattern %q was found in %s (deny rule)\n", v.Rule.Pattern, v.Rule.Scope))
		} else {
			sb.WriteString(fmt.Sprintf("     Pattern %q was not found in %s (require rule)\n", v.Rule.Pattern, v.Rule.Scope))
		}
	}

	return fmt.Errorf("%s", sb.String())
}

// getViolationMessage returns a custom message or generates a default based on rule type.
func getViolationMessage(v RuleViolation) string {
	if v.Rule.Message != "" {
		return v.Rule.Message
	}

	// Default message based on rule type
	if v.Rule.Type == RuleTypeDeny {
		return fmt.Sprintf("Pattern must not match in %s", v.Rule.Scope)
	}

	return fmt.Sprintf("Pattern must match in %s", v.Rule.Scope)
}

// getFirstLine extracts and returns the first line of a commit message.
func getFirstLine(message string) string {
	lines := strings.Split(message, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}

	return ""
}
