package commitmsg

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Test helpers - exported for testing only

// ParseArgsForTesting exposes parseArgs for testing.
func ParseArgsForTesting(config *Config, args []string) (baseRef string, headRef string, err error) {
	return parseArgs(config, args)
}

// ResolveRefOrSHAForTesting exposes resolveRefOrSHA for testing.
func ResolveRefOrSHAForTesting(repo *git.Repository, refOrSHA string) (*object.Commit, error) {
	return resolveRefOrSHA(repo, refOrSHA)
}
