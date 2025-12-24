package commitmsg_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/breml/githooks/internal/hooks/commitmsg"
)

func TestLoadConfig_Valid(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *commitmsg.Config)
	}{
		{
			name: "valid config with single deny rule",
			configYAML: `rules:
  - name: prevent-wip
    type: deny
    scope: title
    pattern: '(?i)wip'
    message: "WIP commits not allowed"
`,
			wantErr: false,
			validate: func(t *testing.T, config *commitmsg.Config) {
				t.Helper()
				if len(config.Rules) != 1 {
					t.Errorf("expected 1 rule, got %d", len(config.Rules))
				}

				if config.Rules[0].Name != "prevent-wip" {
					t.Errorf("expected rule name 'prevent-wip', got %q", config.Rules[0].Name)
				}

				if config.Rules[0].Type != commitmsg.RuleTypeDeny {
					t.Errorf("expected rule type 'deny', got %q", config.Rules[0].Type)
				}

				if config.Rules[0].Scope != commitmsg.ScopeTitle {
					t.Errorf("expected scope 'title', got %q", config.Rules[0].Scope)
				}

				// regex field is unexported, can't check it from _test package
			},
		},
		{
			name: "valid config with require rule",
			configYAML: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by:'
    message: "Commits must be signed off"
`,
			wantErr: false,
			validate: func(t *testing.T, config *commitmsg.Config) {
				t.Helper()
				if config.Rules[0].Type != commitmsg.RuleTypeRequire {
					t.Errorf("expected rule type 'require', got %q", config.Rules[0].Type)
				}

				if config.Rules[0].Scope != commitmsg.ScopeFooter {
					t.Errorf("expected scope 'footer', got %q", config.Rules[0].Scope)
				}
			},
		},
		{
			name: "valid config with multiple rules",
			configYAML: `rules:
  - name: prevent-wip
    type: deny
    scope: title
    pattern: 'wip'
  - name: require-signoff
    type: require
    scope: footer
    pattern: 'Signed-off-by'
  - name: no-emoji
    type: deny
    scope: message
    pattern: '\p{So}'
`,
			wantErr: false,
			validate: func(t *testing.T, config *commitmsg.Config) {
				t.Helper()
				if len(config.Rules) != 3 {
					t.Errorf("expected 3 rules, got %d", len(config.Rules))
				}
			},
		},
		{
			name: "valid config with settings",
			configYAML: `rules:
  - name: test
    type: deny
    scope: title
    pattern: 'test'
settings:
  fail_fast: true
  skip_merge_commits: true
  main_ref: master
  skip_authors:
    - 'renovate\[bot\]'
    - 'dependabot'
`,
			wantErr: false,
			validate: func(t *testing.T, config *commitmsg.Config) {
				t.Helper()
				if !config.Settings.FailFast {
					t.Error("expected FailFast to be true")
				}

				if !config.Settings.SkipMergeCommits {
					t.Error("expected SkipMergeCommits to be true")
				}

				if config.Settings.MainRef != "master" {
					t.Errorf("expected MainRef to be 'master', got %q", config.Settings.MainRef)
				}

				if len(config.Settings.SkipAuthors) != 2 {
					t.Errorf("expected 2 skip_authors, got %d", len(config.Settings.SkipAuthors))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runLoadConfigTest(t, tt)
		})
	}
}

func TestLoadConfig_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *commitmsg.Config)
	}{
		{
			name: "invalid YAML",
			configYAML: `rules:
  - name: test
    type: deny
  invalid yaml here
`,
			wantErr:     true,
			errContains: "failed to parse config YAML",
		},
		{
			name:        "no rules defined",
			configYAML:  `rules: []`,
			wantErr:     true,
			errContains: "no rules defined",
		},
		{
			name: "missing rule name",
			configYAML: `rules:
  - type: deny
    scope: title
    pattern: 'test'
`,
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "invalid rule type",
			configYAML: `rules:
  - name: test
    type: invalid
    scope: title
    pattern: 'test'
`,
			wantErr:     true,
			errContains: "type must be 'deny' or 'require'",
		},
		{
			name: "invalid scope",
			configYAML: `rules:
  - name: test
    type: deny
    scope: invalid
    pattern: 'test'
`,
			wantErr:     true,
			errContains: "scope must be 'title', 'body', 'footer', or 'message'",
		},
		{
			name: "missing pattern",
			configYAML: `rules:
  - name: test
    type: deny
    scope: title
`,
			wantErr:     true,
			errContains: "pattern is required",
		},
		{
			name: "invalid regex pattern",
			configYAML: `rules:
  - name: test
    type: deny
    scope: title
    pattern: '(?i[invalid'
`,
			wantErr:     true,
			errContains: "invalid regex pattern",
		},
		{
			name: "invalid skip_authors pattern",
			configYAML: `rules:
  - name: test
    type: deny
    scope: title
    pattern: 'test'
settings:
  skip_authors:
    - '[invalid'
`,
			wantErr:     true,
			errContains: "skip_authors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runLoadConfigTest(t, tt)
		})
	}
}

func runLoadConfigTest(t *testing.T, tt struct {
	name        string
	configYAML  string
	wantErr     bool
	errContains string
	validate    func(*testing.T, *commitmsg.Config)
},
) {
	t.Helper()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Write the config file
	configPath := filepath.Join(tmpDir, commitmsg.DefaultConfigFile)
	err := os.WriteFile(configPath, []byte(tt.configYAML), 0o644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Load the config
	config, err := commitmsg.LoadConfig(tmpDir)

	if tt.wantErr {
		if err == nil {
			t.Errorf("expected error, got nil")
			return
		}

		if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
			t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
		}

		return
	}

	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	if tt.validate != nil {
		tt.validate(t, config)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := commitmsg.LoadConfig(tmpDir)
	if err == nil {
		t.Error("expected error for missing config file, got nil")
	}

	if !contains(err.Error(), "config file not found") {
		t.Errorf("expected 'config file not found' error, got: %v", err)
	}
}

func TestValidateConfig_RegexCaching(t *testing.T) {
	// Test that LoadConfig compiles regex patterns
	// (regex field is unexported, so we test indirectly via LoadConfig)
	tmpDir := t.TempDir()

	configYAML := `rules:
  - name: test
    type: deny
    scope: title
    pattern: 'test.*pattern'
`

	configPath := filepath.Join(tmpDir, commitmsg.DefaultConfigFile)
	err := os.WriteFile(configPath, []byte(configYAML), 0o644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := commitmsg.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Just verify config loaded successfully
	if len(config.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(config.Rules))
	}
}

func TestRuleType_Values(t *testing.T) {
	tests := []struct {
		value commitmsg.RuleType
		valid bool
	}{
		{commitmsg.RuleTypeDeny, true},
		{commitmsg.RuleTypeRequire, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.value), func(t *testing.T) {
			tmpDir := t.TempDir()

			configYAML := fmt.Sprintf(`rules:
  - name: test
    type: %s
    scope: title
    pattern: 'test'
`, tt.value)

			configPath := filepath.Join(tmpDir, commitmsg.DefaultConfigFile)
			_ = os.WriteFile(configPath, []byte(configYAML), 0o644)

			_, err := commitmsg.LoadConfig(tmpDir)
			if tt.valid && err != nil {
				t.Errorf("expected valid rule type %q to pass, got error: %v", tt.value, err)
			}

			if !tt.valid && err == nil {
				t.Errorf("expected invalid rule type %q to fail, got nil error", tt.value)
			}
		})
	}
}

func TestScope_Values(t *testing.T) {
	tests := []struct {
		value commitmsg.Scope
		valid bool
	}{
		{commitmsg.ScopeTitle, true},
		{commitmsg.ScopeBody, true},
		{commitmsg.ScopeFooter, true},
		{commitmsg.ScopeMessage, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.value), func(t *testing.T) {
			tmpDir := t.TempDir()

			configYAML := fmt.Sprintf(`rules:
  - name: test
    type: deny
    scope: %s
    pattern: 'test'
`, tt.value)

			configPath := filepath.Join(tmpDir, commitmsg.DefaultConfigFile)
			_ = os.WriteFile(configPath, []byte(configYAML), 0o644)

			_, err := commitmsg.LoadConfig(tmpDir)
			if tt.valid && err != nil {
				t.Errorf("expected valid scope %q to pass, got error: %v", tt.value, err)
			}

			if !tt.valid && err == nil {
				t.Errorf("expected invalid scope %q to fail, got nil error", tt.value)
			}
		})
	}
}

func TestValidateConfig_UnicodePattern(t *testing.T) {
	tmpDir := t.TempDir()

	configYAML := `rules:
  - name: no-emoji
    type: deny
    scope: message
    pattern: '\p{So}'
`

	configPath := filepath.Join(tmpDir, commitmsg.DefaultConfigFile)
	err := os.WriteFile(configPath, []byte(configYAML), 0o644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := commitmsg.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("Unicode pattern should be valid: %v", err)
	}

	// Just verify config loaded successfully
	if len(config.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(config.Rules))
	}
}

func contains(s, substr string) bool {
	return regexp.MustCompile(regexp.QuoteMeta(substr)).MatchString(s)
}
