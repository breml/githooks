package commitmsg_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/breml/githooks/internal/hooks/commitmsg"
)

func TestStripCommentLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no comments",
			input: "feat: add feature\n\nSome body text",
			want:  "feat: add feature\n\nSome body text",
		},
		{
			name:  "comment only",
			input: "# Please enter a commit message",
			want:  "",
		},
		{
			name:  "mixed lines",
			input: "feat: add feature\n# Please enter a commit message\n# Changes:\n#\tmodified: file.go\n",
			want:  "feat: add feature\n",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "comment at end",
			input: "feat: add feature\n\nSigned-off-by: Dev <dev@example.com>\n# On branch main\n",
			want:  "feat: add feature\n\nSigned-off-by: Dev <dev@example.com>\n",
		},
		{
			name:  "line with hash in body is preserved",
			input: "feat: add feature\n\nSee issue #42 for context",
			want:  "feat: add feature\n\nSee issue #42 for context",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := commitmsg.StripCommentLinesForTesting(tc.input)
			if got != tc.want {
				t.Errorf("StripCommentLines() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsMergeInProgress(t *testing.T) {
	tmpDir, repo, hashes := createTestRepo(t, []commit{
		{message: "Initial commit", files: map[string]string{"file1.txt": "content1"}},
	})
	t.Chdir(tmpDir)

	if commitmsg.IsMergeInProgressForTesting(repo) {
		t.Error("IsMergeInProgress() = true before MERGE_HEAD set, want false")
	}

	// Simulate a merge in progress by writing MERGE_HEAD
	mergeRef := plumbing.NewHashReference(plumbing.ReferenceName("MERGE_HEAD"), hashes[0])
	err := repo.Storer.SetReference(mergeRef)
	if err != nil {
		t.Fatalf("failed to set MERGE_HEAD: %v", err)
	}

	if !commitmsg.IsMergeInProgressForTesting(repo) {
		t.Error("IsMergeInProgress() = false after MERGE_HEAD set, want true")
	}
}

func TestRunCommitMsgHook(t *testing.T) {
	tests := []struct {
		name          string
		config        string
		messageInFile string
		wantErr       bool
		description   string
	}{
		{
			name:          "valid message passes",
			config:        defaultWIPConfig,
			messageInFile: "feat: add feature\n",
			wantErr:       false,
			description:   "Clean commit message should pass",
		},
		{
			name:          "WIP message rejected",
			config:        defaultWIPConfig,
			messageInFile: "WIP: debugging\n",
			wantErr:       true,
			description:   "WIP commit message should be rejected",
		},
		{
			name:          "message with git comments stripped before linting",
			config:        defaultWIPConfig,
			messageInFile: "feat: add feature\n# Please enter a commit message\n# On branch main\n",
			wantErr:       false,
			description:   "Git comment lines should be stripped before linting",
		},
		{
			name: "require rule passes when pattern present",
			config: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by:'
`,
			messageInFile: "feat: add feature\n\nSigned-off-by: Dev <dev@example.com>\n",
			wantErr:       false,
			description:   "Required sign-off present should pass",
		},
		{
			name: "require rule fails when pattern absent",
			config: `rules:
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by:'
`,
			messageInFile: "feat: add feature\n",
			wantErr:       true,
			description:   "Missing required sign-off should fail",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, _, _ := createTestRepo(t, nil)
			writeConfigFile(t, tmpDir, tc.config)
			t.Chdir(tmpDir)

			// Write commit message to a temp file (simulating git's invocation)
			msgFile := filepath.Join(tmpDir, "COMMIT_EDITMSG")
			writeErr := os.WriteFile(msgFile, []byte(tc.messageInFile), 0o644)
			if writeErr != nil {
				t.Fatalf("failed to write message file: %v", writeErr)
			}

			// Run with the file path as args[1] — auto-detects commit-msg hook mode
			err := commitmsg.Run(strings.NewReader(""), []string{"commit-msg-lint", msgFile})

			if (err != nil) != tc.wantErr {
				t.Errorf("Run() error = %v, wantErr %v (%s)", err, tc.wantErr, tc.description)
			}
		})
	}
}

func TestRunCommitMsgHookSkipsMergeCommit(t *testing.T) {
	tmpDir, repo, hashes := createTestRepo(t, []commit{
		{message: "Initial commit", files: map[string]string{"file1.txt": "content1"}},
	})
	writeConfigFile(t, tmpDir, `rules:
  - name: prevent-wip
    type: deny
    scope: title
    pattern: '(?i)(?:^|[\s\(\)])(wip)(?:[\s\(\):]|$)'
    message: "WIP commits are not allowed"
settings:
  skip_merge_commits: true
`)
	t.Chdir(tmpDir)

	// Simulate merge in progress
	mergeRef := plumbing.NewHashReference(plumbing.ReferenceName("MERGE_HEAD"), hashes[0])
	err := repo.Storer.SetReference(mergeRef)
	if err != nil {
		t.Fatalf("failed to set MERGE_HEAD: %v", err)
	}

	msgFile := filepath.Join(tmpDir, "COMMIT_EDITMSG")
	writeErr := os.WriteFile(msgFile, []byte("Merge branch 'feature' into main\n"), 0o644)
	if writeErr != nil {
		t.Fatalf("failed to write message file: %v", writeErr)
	}

	// Merge commit should be skipped even if message would otherwise trigger a rule
	runErr := commitmsg.Run(strings.NewReader(""), []string{"commit-msg-lint", msgFile})
	if runErr != nil {
		t.Errorf("Run() returned unexpected error for merge commit: %v", runErr)
	}
}

func TestRunPrePushHook(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		commits     []commit
		input       func([]plumbing.Hash) string
		wantErr     bool
		description string
	}{
		{
			name:        "empty stdin passes",
			config:      defaultWIPConfig,
			commits:     nil,
			input:       func(_ []plumbing.Hash) string { return "" },
			wantErr:     false,
			description: "Empty stdin should pass",
		},
		{
			name:   "clean commits pass",
			config: defaultWIPConfig,
			commits: []commit{
				{message: "feat: add feature", files: map[string]string{"file1.txt": "content1"}},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf("refs/heads/feature %s refs/heads/feature %s\n",
					hashes[0].String(), gitZeroHash)
			},
			wantErr:     false,
			description: "Clean commits should pass",
		},
		{
			name:   "WIP commit rejected",
			config: defaultWIPConfig,
			commits: []commit{
				{message: "WIP: debugging", files: map[string]string{"file1.txt": "content1"}},
			},
			input: func(hashes []plumbing.Hash) string {
				return fmt.Sprintf("refs/heads/feature %s refs/heads/feature %s\n",
					hashes[0].String(), gitZeroHash)
			},
			wantErr:     true,
			description: "WIP commit should be rejected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, _, hashes := createTestRepo(t, tc.commits)
			writeConfigFile(t, tmpDir, tc.config)
			t.Chdir(tmpDir)

			input := tc.input(hashes)
			err := commitmsg.RunPrePushHook(strings.NewReader(input), []string{"commit-msg-lint-prepush"})

			if (err != nil) != tc.wantErr {
				t.Errorf("RunPrePushHook() error = %v, wantErr %v (%s)", err, tc.wantErr, tc.description)
			}
		})
	}
}

func TestAutoDetect(t *testing.T) {
	tmpDir, _, _ := createTestRepo(t, []commit{
		{message: "Initial commit", files: map[string]string{"file1.txt": "content1"}},
	})
	writeConfigFile(t, tmpDir, defaultWIPConfig)
	t.Chdir(tmpDir)

	// Write a clean commit message file
	msgFile := filepath.Join(tmpDir, "COMMIT_EDITMSG")
	err := os.WriteFile(msgFile, []byte("feat: add feature\n"), 0o644)
	if err != nil {
		t.Fatalf("failed to write message file: %v", err)
	}

	t.Run("file arg triggers commit-msg mode", func(t *testing.T) {
		// args[1] is an existing file → commit-msg hook mode
		err := commitmsg.Run(strings.NewReader(""), []string{"commit-msg-lint", msgFile})
		if err != nil {
			t.Errorf("Run() returned unexpected error in commit-msg mode: %v", err)
		}
	})

	t.Run("non-file arg triggers pre-push mode", func(t *testing.T) {
		// args[1] is "origin" (not a file) → pre-push hook mode; empty stdin = no refs = pass
		err := commitmsg.Run(
			strings.NewReader(""),
			[]string{"commit-msg-lint", "origin", "https://example.com/repo.git"},
		)
		if err != nil {
			t.Errorf("Run() returned unexpected error in pre-push mode: %v", err)
		}
	})

	t.Run("no args triggers pre-push mode", func(t *testing.T) {
		// No args → pre-push hook mode; empty stdin = no refs = pass
		err := commitmsg.Run(strings.NewReader(""), nil)
		if err != nil {
			t.Errorf("Run() returned unexpected error with no args: %v", err)
		}
	})
}
