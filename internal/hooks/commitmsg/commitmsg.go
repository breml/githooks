package commitmsg

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

const (
	minRefFields     = 4
	commitRangeParts = 2

	gitZeroHash    = "0000000000000000000000000000000000000000"
	defaultMainRef = "main"
	currentDir     = "."
)

// isKnownCommitMsgBasename reports whether name is one of the filenames git
// uses when invoking the commit-msg hook. This lets the tool recognise bare
// filenames like "COMMIT_EDITMSG" (no directory component) in addition to the
// normal ".git/COMMIT_EDITMSG" path form.
func isKnownCommitMsgBasename(name string) bool {
	switch name {
	case "COMMIT_EDITMSG", "MERGE_MSG", "SQUASH_MSG":
		return true

	default:
		return false
	}
}

// parseArgs parses command-line arguments and returns base and head refs.
// Returns empty strings if no flags are provided (stdin mode).
func parseArgs(config *Config, args []string) (baseRef string, headRef string, err error) {
	// Handle nil or empty args (stdin mode)
	if len(args) == 0 {
		return "", "", nil
	}

	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Don't print default error messages

	var base, head string
	fs.StringVar(&base, "base-ref", "", "Base ref or SHA to compare from")
	fs.StringVar(&head, "head-ref", "", "Head ref or SHA to compare to")

	err = fs.Parse(args[1:])
	if err != nil {
		return "", "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// If no flags provided, return empty strings (stdin mode)
	if base == "" && head == "" {
		return "", "", nil
	}

	// If only head-ref is provided, default base-ref to "main"
	if base == "" && head != "" {
		base = config.Settings.MainRef
	}

	// If only base-ref is provided, error (need head-ref)
	if base != "" && head == "" {
		return "", "", errors.New("--head-ref is required when using --base-ref")
	}

	return base, head, nil
}

// resolveRefOrSHA resolves a ref name or SHA to a commit object.
// Tries as ref first (branches, tags, HEAD), then as SHA.
func resolveRefOrSHA(repo *git.Repository, refOrSHA string) (*object.Commit, error) {
	// Try as ref name first (handles branches, remotes, tags, HEAD, HEAD^, etc.)
	hash, err := repo.ResolveRevision(plumbing.Revision(refOrSHA))
	if err == nil {
		commit, err := repo.CommitObject(*hash)
		if err == nil {
			return commit, nil
		}
	}

	// Try as direct SHA
	commit, err := repo.CommitObject(plumbing.NewHash(refOrSHA))
	if err == nil {
		return commit, nil
	}

	return nil, fmt.Errorf("failed to resolve '%s' as ref or SHA", refOrSHA)
}

// resolveBaseOID determines the base commit OID for computing the commit range.
// For new branches (remoteOID is zero hash), it falls back to the configured main ref.
// For existing branches, it checks whether remoteOID is an ancestor of localOID.
// If not (e.g. after a rebase + force push), it falls back to the configured main ref.
func resolveBaseOID(config *Config, repo *git.Repository, remoteOID string, localOID string) (string, error) {
	if remoteOID == gitZeroHash {
		// New branch, examine all commits since main branch
		mainRef, err := resolveRefOrSHA(repo, config.Settings.MainRef)
		if err != nil {
			return "", fmt.Errorf("failed to resolve main ref: %w", err)
		}

		return mainRef.Hash.String(), nil
	}

	// Check if remoteOID is an ancestor of localOID.
	// If not (e.g. after a rebase + force push), the remote ref
	// is no longer in the local commit graph and cannot be used
	// as the base. Fall back to the configured main ref.
	ancestor, err := isAncestorOf(repo, remoteOID, localOID)
	if err != nil || !ancestor {
		mainRef, resolveErr := resolveRefOrSHA(repo, config.Settings.MainRef)
		if resolveErr != nil {
			return "", fmt.Errorf("failed to resolve main ref: %w", resolveErr)
		}

		return mainRef.Hash.String(), nil
	}

	return remoteOID, nil
}

// runStdinMode reads git pre-push hook input from stdin and validates commits.
func runStdinMode(config *Config, repo *git.Repository, stdin io.Reader) error {
	// Read from stdin - git pre-push hook provides refs via stdin
	scanner := bufio.NewScanner(stdin)

	const (
		stdinPosLocalRef  = 0
		stdinPosLocalOID  = 1
		stdinPosRemoteOID = 3
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < minRefFields {
			continue
		}

		localRef := fields[stdinPosLocalRef]
		localOID := fields[stdinPosLocalOID]
		remoteOID := fields[stdinPosRemoteOID]

		// Handle delete
		if localOID == gitZeroHash {
			continue
		}

		// Determine the base commit for the range
		baseOID, err := resolveBaseOID(config, repo, remoteOID, localOID)
		if err != nil {
			return err
		}

		commitRange := fmt.Sprintf("%s..%s", baseOID, localOID)

		// Check commits in the range
		checkErr := checkCommits(config, repo, commitRange, localRef)
		if checkErr != nil {
			return checkErr
		}
	}

	err := scanner.Err()
	if err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	return nil
}

// validateCommits validates a list of commits against configured rules.
func validateCommits(config *Config, commits []*object.Commit, refName string) error {
	for _, commit := range commits {
		// Skip merge commits if configured
		if config.Settings.SkipMergeCommits != nil && *config.Settings.SkipMergeCommits &&
			len(commit.ParentHashes) > 1 {
			continue
		}

		// Skip by author pattern if configured
		if shouldSkipAuthor(commit.Author.Name, commit.Author.Email, config.Settings.SkipAuthors) {
			continue
		}

		// Parse commit message
		parsed := ParseCommitMessage(commit.Message)

		// Evaluate all rules
		violations := EvaluateRules(config.Rules, parsed)

		if len(violations) > 0 {
			// In fail-fast mode, only show the first violation
			violationsToShow := violations
			if config.Settings.FailFast {
				violationsToShow = violations[:1]
			}

			return formatViolationError(commit, refName, violationsToShow)
		}
	}

	return nil
}

// runArgsMode validates commits between base and head refs/SHAs.
func runArgsMode(config *Config, repo *git.Repository, baseRef string, headRef string) error {
	// Resolve base and head to commits
	baseCommit, err := resolveRefOrSHA(repo, baseRef)
	if err != nil {
		if baseRef == config.Settings.MainRef {
			return fmt.Errorf("%w (hint: use --base-ref to specify a different base)", err)
		}

		return err
	}

	headCommit, err := resolveRefOrSHA(repo, headRef)
	if err != nil {
		return err
	}

	// Get commits in range base..head
	commits, err := getCommitsInRange(repo, baseCommit.Hash.String(), headCommit.Hash.String())
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	// Validate commits
	refName := fmt.Sprintf("%s..%s", baseRef, headRef)
	return validateCommits(config, commits, refName)
}

// stripCommentLines removes lines starting with '#' from a commit message.
// Git adds comment lines (e.g. hints, status) to the commit message file; these must
// be stripped before linting so they do not trigger rule violations.
func stripCommentLines(msg string) string {
	lines := strings.Split(msg, "\n")
	filtered := lines[:0]

	for _, line := range lines {
		if !strings.HasPrefix(line, "#") {
			filtered = append(filtered, line)
		}
	}

	return strings.Join(filtered, "\n")
}

// isMergeInProgress reports whether a merge is currently in progress by checking
// whether the MERGE_HEAD reference exists in the repository.
func isMergeInProgress(repo *git.Repository) bool {
	_, err := repo.Storer.Reference(plumbing.ReferenceName("MERGE_HEAD"))

	return err == nil
}

// runCommitMsgHookMode validates a single commit message read from msgFilePath.
// This is used when the binary is invoked as a git commit-msg hook.
// Note: skip_authors is not evaluated in this mode because the commit author is
// not yet determined at commit-msg hook time.
func runCommitMsgHookMode(config *Config, repo *git.Repository, msgFilePath string) error {
	// Skip merge commits if configured
	if config.Settings.SkipMergeCommits != nil && *config.Settings.SkipMergeCommits && isMergeInProgress(repo) {
		return nil
	}

	msgBytes, err := os.ReadFile(msgFilePath)
	if err != nil {
		return fmt.Errorf("failed to read commit message file: %w", err)
	}

	message := stripCommentLines(string(msgBytes))
	parsed := ParseCommitMessage(message)
	violations := EvaluateRules(config.Rules, parsed)

	if len(violations) == 0 {
		return nil
	}

	violationsToShow := violations
	if config.Settings.FailFast {
		violationsToShow = violations[:1]
	}

	return formatMessageViolationError(msgFilePath, violationsToShow)
}

// Run validates commit messages.
// Mode is auto-detected from the arguments:
//   - If --base-ref / --head-ref flags are present: CI mode (validate commit range)
//   - If args[1] is an existing file: commit-msg hook mode (validate that file)
//   - Otherwise: pre-push hook mode (read refs from stdin)
func Run(stdin io.Reader, args []string) error {
	// Load configuration from .commit-msg-lint.yml
	config, err := LoadConfig(currentDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply default for main_ref if not explicitly set
	if config.Settings.MainRef == "" {
		config.Settings.MainRef = defaultMainRef
	}

	// Parse command-line arguments
	baseRef, headRef, err := parseArgs(config, args)
	if err != nil {
		return err
	}

	// Apply default for skip_merge_commits if not explicitly set in config
	if config.Settings.SkipMergeCommits == nil {
		defaultTrue := true
		config.Settings.SkipMergeCommits = &defaultTrue
	}

	repo, err := git.PlainOpen(currentDir)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Dispatch based on input mode
	if headRef != "" {
		// CI mode: validate between base and head refs
		return runArgsMode(config, repo, baseRef, headRef)
	}

	// Auto-detect commit-msg hook mode: git always passes the commit message file as a
	// path with a directory component (e.g. .git/COMMIT_EDITMSG). The basename may also
	// match a known git commit message filename for invocations without a path separator.
	// Remote names used by pre-push hooks (e.g. "origin") have neither property.
	if len(args) >= 2 && (filepath.Dir(args[1]) != currentDir || isKnownCommitMsgBasename(filepath.Base(args[1]))) {
		info, statErr := os.Stat(args[1])
		if statErr == nil && info.Mode().IsRegular() {
			return runCommitMsgHookMode(config, repo, args[1])
		}
	}

	// Pre-push hook mode: read from stdin
	return runStdinMode(config, repo, stdin)
}

// RunPrePushHook validates commits from git pre-push hook input on stdin.
// Use this entry point when the binary is explicitly deployed as a pre-push hook,
// bypassing the auto-detection in Run.
func RunPrePushHook(stdin io.Reader, _ []string) error {
	config, err := LoadConfig(currentDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config.Settings.MainRef == "" {
		config.Settings.MainRef = defaultMainRef
	}

	if config.Settings.SkipMergeCommits == nil {
		defaultTrue := true
		config.Settings.SkipMergeCommits = &defaultTrue
	}

	repo, err := git.PlainOpen(currentDir)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	return runStdinMode(config, repo, stdin)
}

// checkCommits validates all commits in the range against configured rules.
func checkCommits(config *Config, repo *git.Repository, commitRange string, ref string) error {
	// Parse the commit range
	var commits []*object.Commit
	var err error
	if strings.Contains(commitRange, "..") {
		// Range format: "oldCommit..newCommit"
		parts := strings.Split(commitRange, "..")
		if len(parts) != commitRangeParts {
			return fmt.Errorf("invalid commit range format: %s", commitRange)
		}

		commits, err = getCommitsInRange(repo, parts[0], parts[1])
	} else {
		// Single commit format: get all commits up to this one
		commits, err = getCommitsUpTo(repo, commitRange)
	}

	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	// Validate commits
	return validateCommits(config, commits, ref)
}

// getCommitsInRange returns all commits between oldCommit and newCommit (exclusive of oldCommit).
func getCommitsInRange(repo *git.Repository, oldCommit string, newCommit string) ([]*object.Commit, error) {
	// Get the new commit
	newHash := plumbing.NewHash(newCommit)
	newCommitObj, err := repo.CommitObject(newHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get new commit %s: %w", newCommit, err)
	}

	// Get the old commit
	oldHash := plumbing.NewHash(oldCommit)
	oldCommitObj, err := repo.CommitObject(oldHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get old commit %s: %w", oldCommit, err)
	}

	// Create a set of old commits to exclude
	oldCommits := map[plumbing.Hash]bool{}
	oldIter := object.NewCommitIterCTime(oldCommitObj, nil, nil)
	err = oldIter.ForEach(func(c *object.Commit) error {
		oldCommits[c.Hash] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate old commits: %w", err)
	}

	// Get commits from new that are not in old
	var commits []*object.Commit
	newIter := object.NewCommitIterCTime(newCommitObj, nil, nil)
	err = newIter.ForEach(func(c *object.Commit) error {
		if !oldCommits[c.Hash] {
			commits = append(commits, c)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate new commits: %w", err)
	}

	return commits, nil
}

// isAncestorOf checks if ancestorHash is an ancestor of (or equal to) descendantHash
// by walking the commit graph from descendant backwards.
func isAncestorOf(repo *git.Repository, ancestorHash string, descendantHash string) (bool, error) {
	descendant, err := repo.CommitObject(plumbing.NewHash(descendantHash))
	if err != nil {
		return false, fmt.Errorf("failed to get descendant commit %s: %w", descendantHash, err)
	}

	ancestor := plumbing.NewHash(ancestorHash)

	found := false
	iter := object.NewCommitIterCTime(descendant, nil, nil)
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == ancestor {
			found = true
			return storer.ErrStop
		}

		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return found, nil
}

// getCommitsUpTo returns all commits up to and including the specified commit.
func getCommitsUpTo(repo *git.Repository, commitHash string) ([]*object.Commit, error) {
	// Get the commit
	hash := plumbing.NewHash(commitHash)
	commitObj, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", commitHash, err)
	}

	// Get all commits up to this one
	var commits []*object.Commit
	iter := object.NewCommitIterCTime(commitObj, nil, nil)
	err = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}
