package commitmsg

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	minRefFields     = 4
	commitRangeParts = 2

	gitZeroHash    = "0000000000000000000000000000000000000000"
	defaultMainRef = "main"
)

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

// runStdinMode reads git pre-push hook input from stdin and validates commits.
func runStdinMode(config *Config, repo *git.Repository, stdin io.Reader) error {
	// Read from stdin - git pre-push hook provides refs via stdin
	scanner := bufio.NewScanner(stdin)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < minRefFields {
			continue
		}

		localRef := fields[0]
		localOID := fields[1]
		remoteOID := fields[3]

		// Handle delete
		if localOID == gitZeroHash {
			continue
		}

		// Determine the range of commits to check
		var commitRange string
		if remoteOID == gitZeroHash {
			// New branch, examine all commits since main branch
			mainRef, err := resolveRefOrSHA(repo, config.Settings.MainRef)
			if err != nil {
				return fmt.Errorf("failed to resolve main ref: %w", err)
			}

			remoteOID = mainRef.Hash.String()
		}

		// Update to existing branch, examine new commits
		commitRange = fmt.Sprintf("%s..%s", remoteOID, localOID)

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
		if config.Settings.SkipMergeCommits && len(commit.ParentHashes) > 1 {
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
			return formatViolationError(commit, refName, violations, config.Settings.FailFast)
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

// Run reads git pre-push hook input from stdin and validates commit messages.
// If args contains CLI flags, it validates the specified commit range instead.
func Run(stdin io.Reader, args []string) error {
	// Load configuration from .commit-msg-lint.yml
	config, err := LoadConfig(".")
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

	// Apply default for skip_merge_commits if not explicitly set
	if !config.Settings.SkipMergeCommits {
		config.Settings.SkipMergeCommits = true
	}

	repo, err := git.PlainOpen(".")
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Dispatch based on input mode
	if headRef != "" {
		// CLI mode: validate between base and head refs
		return runArgsMode(config, repo, baseRef, headRef)
	}

	// Stdin mode: read from stdin (pre-push hook)
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
	oldCommits := make(map[plumbing.Hash]bool)
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
