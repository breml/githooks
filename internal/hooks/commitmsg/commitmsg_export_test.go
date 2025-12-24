package commitmsg

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Test helpers - exported for testing only

// ParseArgs exposes parseArgs for testing.
func ParseArgs(config *Config, args []string) (string, string, error) {
	return parseArgs(config, args)
}

// ResolveRefOrSHA exposes resolveRefOrSHA for testing.
func ResolveRefOrSHA(repo *git.Repository, refOrSHA string) (*object.Commit, error) {
	return resolveRefOrSHA(repo, refOrSHA)
}
