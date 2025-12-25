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

type commit struct {
	message string
	files   map[string]string
}

// Helper function to create a test repository with commits.
func createTestRepo(
	t *testing.T,
	commits []commit,
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

	// Create an initial base commit for main branch to point to
	baseFilePath := filepath.Join(tmpDir, ".gitkeep")
	err = os.WriteFile(baseFilePath, []byte(""), 0o644)
	if err != nil {
		t.Fatalf("failed to write base file: %v", err)
	}

	_, err = worktree.Add(".gitkeep")
	if err != nil {
		t.Fatalf("failed to add base file: %v", err)
	}

	baseHash, err := worktree.Commit("Initial repository setup", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to create base commit: %v", err)
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

	// Create a 'main' branch reference pointing to the base commit
	// This is needed for tests that expect a main branch to exist
	mainRef := plumbing.NewHashReference("refs/heads/main", baseHash)
	err = repo.Storer.SetReference(mainRef)
	if err != nil {
		t.Fatalf("failed to create main branch: %v", err)
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
		name        string
		config      string
		commits     []commit
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
			commits: []commit{
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
					hashes[0].String(),
				)
			},
			wantErr:     false,
			description: "Should pass when new branch has no WIP commits",
		},
		{
			name:   "new branch with WIP commit",
			config: defaultWIPConfig,
			commits: []commit{
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
					hashes[0].String(),
				)
			},
			wantErr:     true,
			description: "Should detect WIP commits when pushing new branch",
		},
		{
			name:   "branch update without WIP commits",
			config: defaultWIPConfig,
			commits: []commit{
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
			commits: []commit{
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
			name:   "multiple refs in single push - all clean",
			config: defaultWIPConfig,
			commits: []commit{
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
			commits: []commit{
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
			commits: []commit{
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
		{
			name: "require rule - signoff missing",
			config: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by:'
`,
			commits: []commit{
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
			commits: []commit{
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
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var (
				tmpDir string
				hashes []plumbing.Hash
			)

			// Create test repo
			tmpDir, _, hashes = createTestRepo(t, testCase.commits)

			// Write config file
			writeConfigFile(t, tmpDir, testCase.config)

			// Change to test repo directory
			t.Chdir(tmpDir)

			// Generate input using the hash function
			input := testCase.input(hashes)

			// Run the test
			reader := strings.NewReader(input)
			err := commitmsg.Run(reader, nil)

			// Check error expectation
			if (err != nil) != testCase.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, testCase.wantErr)
				return
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantBase    string
		wantHead    string
		wantErr     bool
		description string
	}{
		{
			name:        "no flags - stdin mode",
			args:        []string{"commit-msg-lint"},
			wantBase:    "",
			wantHead:    "",
			wantErr:     false,
			description: "Empty args should return empty strings for stdin mode",
		},
		{
			name:        "both flags provided",
			args:        []string{"commit-msg-lint", "--base-ref", "main", "--head-ref", "feature"},
			wantBase:    "main",
			wantHead:    "feature",
			wantErr:     false,
			description: "Should parse both flags correctly",
		},
		{
			name:        "only head-ref - defaults base to main",
			args:        []string{"commit-msg-lint", "--head-ref", "feature"},
			wantBase:    "main",
			wantHead:    "feature",
			wantErr:     false,
			description: "Should default base-ref to main when only head-ref provided",
		},
		{
			name:        "only base-ref - error",
			args:        []string{"commit-msg-lint", "--base-ref", "main"},
			wantBase:    "",
			wantHead:    "",
			wantErr:     true,
			description: "Should error when only base-ref is provided",
		},
		{
			name:        "SHA values instead of refs",
			args:        []string{"commit-msg-lint", "--base-ref", "abc123def456", "--head-ref", "789abc012def"},
			wantBase:    "abc123def456",
			wantHead:    "789abc012def",
			wantErr:     false,
			description: "Should accept SHA values in place of ref names",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Use the private parseArgs function through exported test helper function.
			base, head, err := commitmsg.ParseArgsForTesting(&commitmsg.Config{
				Settings: commitmsg.Settings{
					MainRef: "main",
				},
			}, testCase.args)

			if (err != nil) != testCase.wantErr {
				t.Errorf("parseArgs() error = %v, wantErr %v", err, testCase.wantErr)
				return
			}

			if base != testCase.wantBase {
				t.Errorf("parseArgs() base = %v, want %v", base, testCase.wantBase)
			}

			if head != testCase.wantHead {
				t.Errorf("parseArgs() head = %v, want %v", head, testCase.wantHead)
			}
		})
	}
}

func TestResolveRefOrSHA(t *testing.T) {
	// Create a test repository with branches
	commits := []commit{
		{
			message: "Initial commit",
			files:   map[string]string{"file1.txt": "content1"},
		},
		{
			message: "Second commit",
			files:   map[string]string{"file2.txt": "content2"},
		},
	}

	tmpDir, repo, hashes := createTestRepo(t, commits)
	t.Chdir(tmpDir)

	// Create a branch pointing to the second commit
	headRef, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	tests := []struct {
		name        string
		refOrSHA    string
		wantHash    plumbing.Hash
		wantErr     bool
		description string
	}{
		{
			name:        "resolve HEAD",
			refOrSHA:    "HEAD",
			wantHash:    hashes[1],
			wantErr:     false,
			description: "Should resolve HEAD to latest commit",
		},
		{
			name:        "resolve by SHA",
			refOrSHA:    hashes[0].String(),
			wantHash:    hashes[0],
			wantErr:     false,
			description: "Should resolve direct SHA",
		},
		{
			name:        "resolve current branch",
			refOrSHA:    headRef.Name().Short(),
			wantHash:    hashes[1],
			wantErr:     false,
			description: "Should resolve branch name",
		},
		{
			name:        "invalid ref",
			refOrSHA:    "nonexistent",
			wantHash:    plumbing.Hash{},
			wantErr:     true,
			description: "Should error on invalid ref",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Use the private resolveRefOrSHA function through exported test helper function.
			commit, err := commitmsg.ResolveRefOrSHAForTesting(repo, testCase.refOrSHA)

			if (err != nil) != testCase.wantErr {
				t.Errorf("resolveRefOrSHA() error = %v, wantErr %v", err, testCase.wantErr)
				return
			}

			if !testCase.wantErr && commit.Hash != testCase.wantHash {
				t.Errorf("resolveRefOrSHA() hash = %v, want %v", commit.Hash, testCase.wantHash)
			}
		})
	}
}

func TestRunWithArgs(t *testing.T) {
	// Create a test repository with clean and WIP commits
	commits := []commit{
		{
			message: "Initial commit",
			files:   map[string]string{"file1.txt": "content1"},
		},
		{
			message: "feat: add feature",
			files:   map[string]string{"file2.txt": "content2"},
		},
		{
			message: "WIP: debugging",
			files:   map[string]string{"file3.txt": "content3"},
		},
	}

	tmpDir, _, hashes := createTestRepo(t, commits)

	// Write WIP prevention config
	writeConfigFile(t, tmpDir, defaultWIPConfig)

	// Change to test repo directory
	t.Chdir(tmpDir)

	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		description string
	}{
		{
			name: "validate clean range with refs",
			args: []string{
				"commit-msg-lint",
				"--base-ref",
				hashes[0].String(),
				"--head-ref",
				hashes[1].String(),
			},
			wantErr:     false,
			description: "Should pass when range has no WIP commits",
		},
		{
			name: "validate range with WIP commit",
			args: []string{
				"commit-msg-lint",
				"--base-ref",
				hashes[1].String(),
				"--head-ref",
				hashes[2].String(),
			},
			wantErr:     true,
			description: "Should fail when range contains WIP commit",
		},
		{
			name:        "validate with HEAD",
			args:        []string{"commit-msg-lint", "--base-ref", hashes[1].String(), "--head-ref", "HEAD"},
			wantErr:     true,
			description: "Should resolve HEAD and detect WIP commit",
		},
		{
			name:        "validate with default main base",
			args:        []string{"commit-msg-lint", "--head-ref", hashes[1].String()},
			wantErr:     false,
			description: "Should use default main branch (created by createTestRepo)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := commitmsg.Run(strings.NewReader(""), testCase.args)

			if (err != nil) != testCase.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, testCase.wantErr)
			}
		})
	}
}
