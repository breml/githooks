package commitmsg_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/breml/githooks/internal/hooks/commitmsg"
)

// Helper function to create a test repository with commits.
func createTestRepo(
	t *testing.T,
	commits []struct {
		message string
		files   map[string]string
	},
) (string, *git.Repository, []plumbing.Hash) {
	t.Helper()

	// Create temporary directory
	tmpDir := t.TempDir()

	// Initialize repository
	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	hashes := make([]plumbing.Hash, 0, len(commits))

	// Create commits
	for commitIdx, commit := range commits {
		// Create or modify files
		for filename, content := range commit.files {
			filePath := filepath.Join(tmpDir, filename)
			writeErr := os.WriteFile(filePath, []byte(content), 0o644)
			if writeErr != nil {
				t.Fatalf("failed to write file %s: %v", filename, writeErr)
			}

			_, addErr := worktree.Add(filename)
			if addErr != nil {
				t.Fatalf("failed to add file %s: %v", filename, addErr)
			}
		}

		// Commit

		hash, commitErr := worktree.Commit(commit.message, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now().Add(time.Duration(commitIdx) * time.Minute),
			},
		})
		if commitErr != nil {
			t.Fatalf("failed to commit: %v", commitErr)
		}

		hashes = append(hashes, hash)
	}

	return tmpDir, repo, hashes
}

// Helper function to create a test config file.
func writeConfigFile(t *testing.T, dir string, config string) {
	t.Helper()

	configPath := filepath.Join(dir, commitmsg.DefaultConfigFile)
	err := os.WriteFile(configPath, []byte(config), 0o644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
}

const gitZeroHash = "0000000000000000000000000000000000000000"

// Default WIP prevention config (backward compatible).
const defaultWIPConfig = `rules:
  - name: prevent-wip
    type: deny
    scope: title
    pattern: '(?i)(?:^|[\s\(\)])(wip)(?:[\s\(\):]|$)'
    message: "WIP commits are not allowed"
`

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		commits []struct {
			message string
			files   map[string]string
		}
		input       func([]plumbing.Hash) string
		wantErr     bool
		description string
	}{
		{
			name:        "empty input",
			config:      defaultWIPConfig,
			commits:     nil,
			input:       func(_ []plumbing.Hash) string { return "" },
			wantErr:     false,
			description: "Should handle empty stdin without error",
		},
		{
			name:        "single blank line",
			config:      defaultWIPConfig,
			commits:     nil,
			input:       func(_ []plumbing.Hash) string { return "\n" },
			wantErr:     false,
			description: "Should skip blank lines",
		},
		{
			name:        "multiple blank lines",
			config:      defaultWIPConfig,
			commits:     nil,
			input:       func(_ []plumbing.Hash) string { return "\n\n\n" },
			wantErr:     false,
			description: "Should skip all blank lines",
		},
		{
			name:        "invalid format - too few fields",
			config:      defaultWIPConfig,
			commits:     nil,
			input:       func(_ []plumbing.Hash) string { return "refs/heads/main abc123\n" },
			wantErr:     false,
			description: "Should skip lines with too few fields",
		},
		{
			name:    "delete operation",
			config:  defaultWIPConfig,
			commits: nil,
			input: func(_ []plumbing.Hash) string {
				return fmt.Sprintf("refs/heads/main %s refs/heads/main abc123def456\n", gitZeroHash)
			},
			wantErr:     false,
			description: "Should skip delete operations (local OID is zero hash)",
		},
		{
			name:   "new branch without WIP commits",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "Add feature",
					files:   map[string]string{"file2.txt": "content2"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/feature %s refs/heads/feature %s\n",
					hashes[1].String(),
					gitZeroHash,
				)
			},
			wantErr:     false,
			description: "Should pass when new branch has no WIP commits",
		},
		{
			name:   "new branch with WIP commit",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "WIP: debugging issue",
					files:   map[string]string{"file2.txt": "content2"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/feature %s refs/heads/feature %s\n",
					hashes[1].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect WIP commits when pushing new branch",
		},
		{
			name:   "branch update without WIP commits",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "Add feature",
					files:   map[string]string{"file2.txt": "content2"},
				},
				{
					message: "Fix bug",
					files:   map[string]string{"file3.txt": "content3"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/main %s refs/heads/main %s\n",
					hashes[2].String(),
					hashes[1].String(),
				)
			},
			wantErr:     false,
			description: "Should pass when branch update has no WIP commits",
		},
		{
			name:   "branch update with WIP commit",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "Add feature",
					files:   map[string]string{"file2.txt": "content2"},
				},
				{
					message: "WIP: incomplete feature",
					files:   map[string]string{"file3.txt": "content3"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/main %s refs/heads/main %s\n",
					hashes[2].String(),
					hashes[1].String(),
				)
			},
			wantErr:     true,
			description: "Should detect WIP commits in branch update",
		},
		{
			name:   "lowercase wip format",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "wip: testing lowercase",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect lowercase 'wip:' format",
		},
		{
			name:   "uppercase WIP format",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "WIP: testing uppercase",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect uppercase 'WIP:' format",
		},
		{
			name:   "WIP in middle of message",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "This is a WIP commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect WIP in middle of commit message",
		},
		{
			name:   "WIP in parentheses",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Fix issue (WIP)",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect WIP in parentheses",
		},
		{
			name:   "mixed case WIP",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "WiP: testing mixed case",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect mixed case WIP",
		},
		{
			name:   "word containing wip should not match",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Swipe feature implementation",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     false,
			description: "Should not match 'wip' as part of another word",
		},
		{
			name:   "multiple refs in single push - all clean",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "Feature A",
					files:   map[string]string{"file2.txt": "content2"},
				},
				{
					message: "Feature B",
					files:   map[string]string{"file3.txt": "content3"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/feature-a %s refs/heads/feature-a %s\n"+
						"refs/heads/feature-b %s refs/heads/feature-b %s\n",
					hashes[1].String(),
					gitZeroHash,
					hashes[2].String(),
					gitZeroHash,
				)
			},
			wantErr:     false,
			description: "Should pass when pushing multiple refs without WIP",
		},
		{
			name:   "multiple refs - one has WIP",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "Feature A complete",
					files:   map[string]string{"file2.txt": "content2"},
				},
				{
					message: "WIP: Feature B incomplete",
					files:   map[string]string{"file3.txt": "content3"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/feature-a %s refs/heads/feature-a %s\n"+
						"refs/heads/feature-b %s refs/heads/feature-b %s\n",
					hashes[1].String(),
					gitZeroHash,
					hashes[2].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect WIP when pushing multiple refs",
		},
		{
			name:   "range with WIP in middle",
			config: defaultWIPConfig,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Initial commit",
					files:   map[string]string{"file1.txt": "content1"},
				},
				{
					message: "WIP: middle commit",
					files:   map[string]string{"file2.txt": "content2"},
				},
				{
					message: "Final commit",
					files:   map[string]string{"file3.txt": "content3"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/main %s refs/heads/main %s\n",
					hashes[2].String(),
					hashes[0].String(),
				)
			},
			wantErr:     true,
			description: "Should detect WIP commit in the middle of a range",
		},
		// New tests for different rule types and scopes
		{
			name: "require rule - signoff present",
			config: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by:'
`,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Add feature\n\nSigned-off-by: Test User <test@example.com>",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     false,
			description: "Should pass when required signoff is present in footer",
		},
		{
			name: "require rule - signoff missing",
			config: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by:'
`,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Add feature",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should fail when required signoff is missing",
		},
		{
			name: "deny rule - fixup commits",
			config: `rules:
  - name: no-fixup
    type: deny
    scope: title
    pattern: '^fixup!'
`,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "fixup! Fix typo",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect fixup commits",
		},
		{
			name: "scope message - checks entire message",
			config: `rules:
  - name: no-emoji
    type: deny
    scope: message
    pattern: '\p{So}'
`,
			commits: []struct {
				message string
				files   map[string]string
			}{
				{
					message: "Add feature\n\nThis adds emoji support 🎉",
					files:   map[string]string{"file1.txt": "content1"},
				},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf(
					"refs/heads/test %s refs/heads/test %s\n",
					hashes[0].String(),
					gitZeroHash,
				)
			},
			wantErr:     true,
			description: "Should detect emoji in message body",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var (
				tmpDir string
				hashes []plumbing.Hash
			)

			// Create test repo if commits are specified
			if testCase.commits != nil {
				tmpDir, _, hashes = createTestRepo(t, testCase.commits)
			} else {
				// Create empty temp dir for config file
				tmpDir = t.TempDir()
			}

			// Write config file
			writeConfigFile(t, tmpDir, testCase.config)

			// Change to test repo directory
			t.Chdir(tmpDir)

			// Generate input using the hash function
			input := testCase.input(hashes)

			// Run the test
			reader := strings.NewReader(input)
			err := commitmsg.Run(reader)

			// Check error expectation
			if (err != nil) != testCase.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, testCase.wantErr)
				return
			}
		})
	}
}
