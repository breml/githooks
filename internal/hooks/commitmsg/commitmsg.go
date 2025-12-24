package commitmsg

import (
	"bufio"
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

	gitZeroHash = "0000000000000000000000000000000000000000"
)

// Run reads git pre-push hook input from stdin and validates commit messages.
func Run(stdin io.Reader) error {
	// Load configuration from .commit-msg-lint.yml
	config, err := LoadConfig(".")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply default for skip_merge_commits if not explicitly set
	if !config.Settings.SkipMergeCommits {
		config.Settings.SkipMergeCommits = true
	}

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
			// New branch, examine all commits
			commitRange = localOID
		} else {
			// Update to existing branch, examine new commits
			commitRange = fmt.Sprintf("%s..%s", remoteOID, localOID)
		}

		// Check commits in the range
		checkErr := checkCommits(config, commitRange, localRef)
		if checkErr != nil {
			return checkErr
		}
	}

	err = scanner.Err()
	if err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}

	return nil
}

// checkCommits validates all commits in the range against configured rules.
func checkCommits(config *Config, commitRange, ref string) error {
	repo, err := git.PlainOpen(".")
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Parse the commit range
	var commits []*object.Commit
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

	// Validate each commit
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
			return formatViolationError(commit, ref, violations, config.Settings.FailFast)
		}
	}

	return nil
}

// getCommitsInRange returns all commits between oldCommit and newCommit (exclusive of oldCommit).
func getCommitsInRange(repo *git.Repository, oldCommit, newCommit string) ([]*object.Commit, error) {
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
