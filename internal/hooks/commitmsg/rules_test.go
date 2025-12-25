package commitmsg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/breml/githooks/internal/hooks/commitmsg"
)

// createRulesFromYAML is a test helper that creates rules by loading a YAML config.
// This ensures regex patterns are properly compiled.
func createRulesFromYAML(t *testing.T, yamlContent string) []commitmsg.Rule {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, commitmsg.DefaultConfigFile)

	err := os.WriteFile(configPath, []byte(yamlContent), 0o644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := commitmsg.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	return config.Rules
}

func TestEvaluateRules(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		message        commitmsg.ParsedCommitMessage
		wantViolations int
		checkViolation func(*testing.T, []commitmsg.RuleViolation)
	}{
		{
			name: "deny rule matches - violation",
			configYAML: `rules:
  - name: prevent-wip
    type: deny
    scope: title
    pattern: '(?i)wip'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "WIP: test",
				Title:  "WIP: test",
				Body:   "",
				Footer: "",
			},
			wantViolations: 1,
			checkViolation: func(t *testing.T, violations []commitmsg.RuleViolation) {
				t.Helper()
				if violations[0].Rule.Name != "prevent-wip" {
					t.Errorf("expected rule name 'prevent-wip', got %q", violations[0].Rule.Name)
				}

				if !violations[0].Matched {
					t.Error("expected Matched to be true for deny rule violation")
				}
			},
		},
		{
			name: "deny rule doesn't match - no violation",
			configYAML: `rules:
  - name: prevent-wip
    type: deny
    scope: title
    pattern: '(?i)wip'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature",
				Title:  "Add feature",
				Body:   "",
				Footer: "",
			},
			wantViolations: 0,
		},
		{
			name: "require rule matches - no violation",
			configYAML: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: 'Signed-off-by'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature\n\nSigned-off-by: John <j@ex.com>",
				Title:  "Add feature",
				Body:   "",
				Footer: "Signed-off-by: John <j@ex.com>",
			},
			wantViolations: 0,
		},
		{
			name: "require rule doesn't match - violation",
			configYAML: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: 'Signed-off-by'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature",
				Title:  "Add feature",
				Body:   "",
				Footer: "",
			},
			wantViolations: 1,
			checkViolation: func(t *testing.T, violations []commitmsg.RuleViolation) {
				t.Helper()
				if violations[0].Matched {
					t.Error("expected Matched to be false for require rule violation")
				}
			},
		},
		{
			name: "scope title - only checks title",
			configYAML: `rules:
  - name: no-wip-title
    type: deny
    scope: title
    pattern: '(?i)wip'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature\n\nThis is a WIP implementation",
				Title:  "Add feature",
				Body:   "This is a WIP implementation",
				Footer: "",
			},
			wantViolations: 0, // WIP is in body, not title
		},
		{
			name: "scope body - only checks body",
			configYAML: `rules:
  - name: no-wip-body
    type: deny
    scope: body
    pattern: '(?i)wip'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature\n\nThis is a WIP implementation",
				Title:  "Add feature",
				Body:   "This is a WIP implementation",
				Footer: "",
			},
			wantViolations: 1, // WIP is in body
		},
		{
			name: "scope footer - only checks footer",
			configYAML: `rules:
  - name: footer-pattern
    type: require
    scope: footer
    pattern: '^Fixes #[0-9]+'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Fix bug\n\nFixes #123\n\nSigned-off-by: John",
				Title:  "Fix bug",
				Body:   "Fixes #123",
				Footer: "Signed-off-by: John",
			},
			wantViolations: 1, // "Fixes #123" is in body, not footer
		},
		{
			name: "scope message - checks entire message",
			configYAML: `rules:
  - name: no-emoji
    type: deny
    scope: message
    pattern: '\p{So}'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature\n\nThis adds emoji support 🎉\n\nFixes #123",
				Title:  "Add feature",
				Body:   "This adds emoji support 🎉",
				Footer: "Fixes #123",
			},
			wantViolations: 1, // Emoji anywhere in message
		},
		{
			name: "multiple rules - all pass",
			configYAML: `rules:
  - name: no-wip
    type: deny
    scope: title
    pattern: '(?i)wip'
  - name: require-signoff
    type: require
    scope: footer
    pattern: 'Signed-off-by'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature\n\nSigned-off-by: John",
				Title:  "Add feature",
				Body:   "",
				Footer: "Signed-off-by: John",
			},
			wantViolations: 0,
		},
		{
			name: "multiple rules - some fail",
			configYAML: `rules:
  - name: no-wip
    type: deny
    scope: title
    pattern: '(?i)wip'
  - name: require-signoff
    type: require
    scope: footer
    pattern: 'Signed-off-by'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "WIP: Add feature",
				Title:  "WIP: Add feature",
				Body:   "",
				Footer: "",
			},
			wantViolations: 2, // Both rules fail
			checkViolation: func(t *testing.T, violations []commitmsg.RuleViolation) {
				t.Helper()
				ruleNames := make(map[string]bool, len(violations))
				for _, v := range violations {
					ruleNames[v.Rule.Name] = true
				}

				if !ruleNames["no-wip"] {
					t.Error("expected 'no-wip' rule to be violated")
				}

				if !ruleNames["require-signoff"] {
					t.Error("expected 'require-signoff' rule to be violated")
				}
			},
		},
		{
			name: "complex regex - conventional commits",
			configYAML: `rules:
  - name: conventional-commits
    type: require
    scope: title
    pattern: '^(feat|fix|docs|style|refactor|perf|test|chore)(\([a-z0-9-]+\))?!?: .+'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "feat(api): Add user authentication",
				Title:  "feat(api): Add user authentication",
				Body:   "",
				Footer: "",
			},
			wantViolations: 0,
		},
		{
			name: "complex regex - conventional commits failure",
			configYAML: `rules:
  - name: conventional-commits
    type: require
    scope: title
    pattern: '^(feat|fix|docs|style|refactor|perf|test|chore)(\([a-z0-9-]+\))?!?: .+'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add user authentication",
				Title:  "Add user authentication",
				Body:   "",
				Footer: "",
			},
			wantViolations: 1,
		},
		{
			name: "empty scope text - require rule fails",
			configYAML: `rules:
  - name: require-footer
    type: require
    scope: footer
    pattern: '.+'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature",
				Title:  "Add feature",
				Body:   "",
				Footer: "",
			},
			wantViolations: 1,
		},
		{
			name: "empty scope text - deny rule passes",
			configYAML: `rules:
  - name: no-bad-word
    type: deny
    scope: body
    pattern: 'badword'
`,
			message: commitmsg.ParsedCommitMessage{
				Raw:    "Add feature",
				Title:  "Add feature",
				Body:   "",
				Footer: "",
			},
			wantViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runEvaluateRulesTest(t, tt)
		})
	}
}

func runEvaluateRulesTest(t *testing.T, tt struct {
	name           string
	configYAML     string
	message        commitmsg.ParsedCommitMessage
	wantViolations int
	checkViolation func(*testing.T, []commitmsg.RuleViolation)
},
) {
	t.Helper()

	rules := createRulesFromYAML(t, tt.configYAML)
	violations := commitmsg.EvaluateRules(rules, tt.message)

	if len(violations) != tt.wantViolations {
		t.Errorf("EvaluateRules() returned %d violations, want %d", len(violations), tt.wantViolations)
		for _, v := range violations {
			t.Logf("  Violation: %s (matched: %v)", v.Rule.Name, v.Matched)
		}
	}

	if tt.checkViolation != nil && len(violations) > 0 {
		tt.checkViolation(t, violations)
	}
}
