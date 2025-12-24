# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository contains a collection of Git hooks written in Go for improved portability. Each hook is implemented as
a standalone Go binary that can be invoked by git hook managers like lefthook.

## Architecture

### Project Structure

The codebase follows standard Go project layout:

- `cmd/<hook-name>/` - Contains the main package and entry point for each hook binary
- `internal/hooks/<hook-name>/` - Contains the core logic and implementation for each hook
- `bin/` - Built binaries for tools and hooks (git-ignored)

### Hook Implementation Pattern

Each hook follows a consistent pattern:

1. The `cmd/<hook-name>/main.go` is a minimal entry point that:
   - Imports the implementation from `internal/hooks/<hook-name>/`
   - Calls `Run()` with appropriate input (e.g., `os.Stdin` and `os.Args` for commit-msg-lint)
   - Handles errors and sets proper exit codes

2. The `internal/hooks/<hook-name>/` package contains:
   - Core logic implementation
   - Unit tests
   - Exported `Run()` function that accepts testable inputs (e.g., `io.Reader`, command-line args)
   - All helper functions are private (lowercase) - only `Run()` and test helpers are exported

This separation enables unit testing of hook logic without executing the actual binary. Tests should focus on testing
the `Run()` function with various inputs rather than testing individual helper functions.

### Existing Hooks

**commit-msg-lint** (pre-push hook and CLI tool):

- **Dual-mode operation:**
  - **Pre-push hook mode:** Reads git pre-push hook input from stdin
    (ref format: `<local ref> <local sha1> <remote ref> <remote sha1>`)
  - **CLI mode:** Accepts `--base-ref` and `--head-ref` flags to validate commits between refs/SHAs for CI/CD usage
- Loads configuration from `.commit-msg-lint.yml` in repository root
- Parses commit messages into three sections: title (first line), body (middle sections), and footer (last section after
  final empty line)
- Evaluates configurable rules against commit message sections
- Supports two rule types:
  - `deny`: Fails if regex pattern matches (e.g., prevent WIP commits)
  - `require`: Fails if regex pattern does NOT match (e.g., require sign-off)
- Supports four scopes: `title`, `body`, `footer`, or `message` (entire commit message)
- Reports all rule violations for each failing commit (configurable via `fail_fast` setting)
- Can skip merge commits and specific authors via configuration
- Uses go-git library to interact with the git repository
- CLI usage:
  - `commit-msg-lint --base-ref main --head-ref feature` - Validate commits between branches
  - `commit-msg-lint --head-ref HEAD` - Validate using default base (main)
  - Both flags accept branch names, tags, or direct SHA values
- Configuration example (`.commit-msg-lint.yml`):

  ```yaml
  rules:
    - name: prevent-wip
      type: deny
      scope: title
      pattern: '(?i)(?:^|[\s\(\)])(wip)(?:[\s\(\):]|$)'
      message: "WIP commits are not allowed"

    - name: require-signoff
      type: require
      scope: footer
      pattern: '^Signed-off-by:'
      message: "Commits must be signed off"

  settings:
    skip_merge_commits: true
  ```

## Development Commands

This project uses [Task](https://taskfile.dev) for command orchestration. All commands are defined in `Taskfile.yml`.

### Initial Setup

```bash
# Install development tools (task, lefthook, golangci-lint, gofumpt, newline-after-block)
task install

# Install git hooks using lefthook
task install-githooks
```

### Build and Test

```bash
# Build all code
task build

# Run all tests
task test

# Format code
task format  # or: task fmt

# Run linters
task lint
```

### Testing Pre-Push Hooks

Since pre-push hooks are invoked by git, test them with:

```bash
git push --dry-run
```

This triggers lefthook which will build and run the hooks without actually pushing.

### Running Individual Tests

```bash
# Run tests for a specific package
go test -v ./internal/hooks/prevent-wip-commits/

# Run a specific test
go test -v -run TestRun ./internal/hooks/prevent-wip-commits/
```

## Code Style and Linting

The project uses:

- `gofumpt` - Stricter gofmt variant for consistent formatting
- `newline-after-block` - Enforces newlines after blocks
- `golangci-lint` - Comprehensive Go linter (configured in `.golangci.yml`)
- `markdownlint-cli2` - Markdown linting (configured in `.markdownlint.yml`)

All formatters and linters run automatically on pre-commit via lefthook and will auto-fix issues when possible.

## Git Hooks (Lefthook)

The project uses lefthook for managing git hooks (configured in `lefthook.yml`):

**pre-commit**: Runs formatters and linters in parallel
**pre-push**: Builds all hooks, runs them (dogfooding), and runs tests

## Adding a New Hook

When adding a new git hook:

1. Create `cmd/<hook-name>/main.go` with minimal entry point
2. Create `internal/hooks/<hook-name>/<hook-name>.go` with:
   - Exported `Run()` function accepting testable inputs (e.g., `io.Reader`)
   - All helper functions as private (lowercase first letter)
3. Create `internal/hooks/<hook-name>/<hook-name>_test.go` with:
   - Table-driven tests that test the `Run()` function with various scenarios
   - Use `createTestRepo` helper to set up git test repositories
   - Focus on testing through the public `Run()` API rather than individual helpers
4. Update `lefthook.yml` to build and invoke the new hook in the appropriate git hook phase
5. Consider the git hook input format (stdin, arguments, environment variables)
6. Use go-git library for git operations rather than shelling out to git commands
