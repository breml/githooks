package commitmsg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile is the name of the configuration file.
const DefaultConfigFile = ".commit-msg-lint.yml"

// RuleType defines the type of rule enforcement.
type RuleType string

const (
	// RuleTypeDeny fails if the pattern matches.
	RuleTypeDeny RuleType = "deny"
	// RuleTypeRequire fails if the pattern does NOT match.
	RuleTypeRequire RuleType = "require"
)

// Scope defines where in the commit message to search.
type Scope string

const (
	// ScopeTitle searches only the first line (title).
	ScopeTitle Scope = "title"
	// ScopeBody searches the middle sections (between title and footer).
	ScopeBody Scope = "body"
	// ScopeFooter searches the last section (after final empty line).
	ScopeFooter Scope = "footer"
	// ScopeMessage searches the complete commit message.
	ScopeMessage Scope = "message"
)

// Config represents the complete configuration for commit message linting.
type Config struct {
	Rules    []Rule   `yaml:"rules"`
	Settings Settings `yaml:"settings,omitempty"`
}

// Rule represents a single linting rule.
type Rule struct {
	Name    string   `yaml:"name"`
	Type    RuleType `yaml:"type"`
	Scope   Scope    `yaml:"scope"`
	Pattern string   `yaml:"pattern"`
	Message string   `yaml:"message,omitempty"`

	// regex is the compiled regular expression (cached, not in YAML)
	regex *regexp.Regexp
}

// Settings contains global configuration options.
type Settings struct {
	FailFast         bool     `yaml:"fail_fast,omitempty"`
	SkipMergeCommits bool     `yaml:"skip_merge_commits,omitempty"`
	SkipAuthors      []string `yaml:"skip_authors,omitempty"`
}

// LoadConfig loads and validates configuration from the specified directory.
func LoadConfig(repoPath string) (*Config, error) {
	configPath := filepath.Join(repoPath, DefaultConfigFile)

	// Check if config file exists
	_, statErr := os.Stat(configPath)
	if os.IsNotExist(statErr) {
		return nil, fmt.Errorf(
			"config file not found: %s\nCreate %s in repository root with linting rules",
			configPath,
			DefaultConfigFile,
		)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate and compile patterns
	err = validateConfig(&config)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

func validateConfig(config *Config) error {
	if len(config.Rules) == 0 {
		return errors.New("no rules defined in config")
	}

	for i := range config.Rules {
		rule := &config.Rules[i]

		// Validate rule name
		if rule.Name == "" {
			return fmt.Errorf("rule %d: name is required", i)
		}

		// Validate rule type
		if rule.Type != RuleTypeDeny && rule.Type != RuleTypeRequire {
			return fmt.Errorf("rule %q: type must be 'deny' or 'require', got %q", rule.Name, rule.Type)
		}

		// Validate scope
		if rule.Scope != ScopeTitle && rule.Scope != ScopeBody &&
			rule.Scope != ScopeFooter && rule.Scope != ScopeMessage {
			return fmt.Errorf(
				"rule %q: scope must be 'title', 'body', 'footer', or 'message', got %q",
				rule.Name,
				rule.Scope,
			)
		}

		// Validate pattern (compile regex)
		if rule.Pattern == "" {
			return fmt.Errorf("rule %q: pattern is required", rule.Name)
		}

		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return fmt.Errorf("rule %q: invalid regex pattern: %w", rule.Name, err)
		}

		// Cache the compiled regex
		rule.regex = re
	}

	// Validate skip_authors patterns
	for i, pattern := range config.Settings.SkipAuthors {
		_, compileErr := regexp.Compile(pattern)
		if compileErr != nil {
			return fmt.Errorf("skip_authors[%d]: invalid regex pattern %q: %w", i, pattern, compileErr)
		}
	}

	return nil
}
