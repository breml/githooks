# githooks

This repo contains a collection of Git hooks written in Go for improved
portability.

## Available Hooks

### commit-msg-lint

A pre-push hook that validates commit messages against configurable rules before allowing pushes to remote repositories.
This helps enforce commit message conventions and prevent problematic commits from being shared.

#### Installation

1. Install lefthook (the git hook manager):

   ```bash
   task install
   ```

2. Install the git hooks:

   ```bash
   task install-githooks
   ```

3. Create a `.commit-msg-lint.yml` configuration file in your repository root (see Configuration below).

#### Configuration

Create a `.commit-msg-lint.yml` file in your repository root with the following structure:

```yaml
rules:
  # Prevent WIP commits from being pushed
  - name: prevent-wip
    type: deny                                              # fail if pattern matches
    scope: title                                            # check only the first line
    pattern: '(?i)(?:^|[\s\(\)])(wip)(?:[\s\(\):]|$)'      # case-insensitive WIP
    message: "WIP commits are not allowed to be pushed"

  # Require Conventional Commits format
  - name: conventional-commits
    type: require                                           # fail if pattern does NOT match
    scope: title
    pattern: '^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\([a-z0-9-]+\))?!?: .+'
    message: "Commit title must follow Conventional Commits format"

  # Require sign-off trailer
  - name: require-signoff
    type: require
    scope: footer
    pattern: '^Signed-off-by: .+ <.+@.+>$'
    message: "Commits must include 'Signed-off-by' trailer (use git commit -s)"

settings:
  fail_fast: false              # Report all violations (true = stop at first)
  skip_merge_commits: true      # Don't validate merge commits
  skip_authors:                 # Skip commits by specific authors (regex)
    - 'renovate\[bot\]'
    - 'dependabot\[bot\]'
```

#### Rule Types

- **`deny`**: Rule fails if the pattern **matches** (use to prevent unwanted patterns)
- **`require`**: Rule fails if the pattern **does NOT match** (use to enforce required patterns)

#### Scopes

Rules can check different parts of the commit message:

- **`title`**: First line of the commit message
- **`body`**: Middle section(s) between title and footer
- **`footer`**: Last section after the final blank line (for trailers like `Signed-off-by`)
- **`message`**: Entire commit message

#### Common Rule Examples

**Prevent WIP commits:**

```yaml
- name: prevent-wip
  type: deny
  scope: title
  pattern: '(?i)(?:^|[\s\(\)])(wip)(?:[\s\(\):]|$)'
  message: "WIP commits are not allowed"
```

**Enforce Conventional Commits:**

```yaml
- name: conventional-commits
  type: require
  scope: title
  pattern: '^(feat|fix|docs|style|refactor|perf|test|chore)(\([a-z0-9-]+\))?!?: .+'
  message: "Use Conventional Commits format (e.g., 'feat: add feature')"
```

**Require issue references:**

```yaml
- name: require-issue-ref
  type: require
  scope: message
  pattern: '(?i)(close[sd]?|fix(e[sd])?|resolve[sd]?|ref):?\s*#\d+'
  message: "Commit must reference an issue (e.g., 'Fixes #123')"
```

**Enforce title length:**

```yaml
- name: title-max-length
  type: deny
  scope: title
  pattern: '^.{73,}'
  message: "Commit title must not exceed 72 characters"
```

**Prevent fixup commits:**

```yaml
- name: no-fixup
  type: deny
  scope: title
  pattern: '^fixup!'
  message: "Fixup commits should be squashed before pushing"
```

#### Testing

Test your hook configuration by attempting a push:

```bash
# Dry run (doesn't actually push)
git push --dry-run

# Or perform an actual push
git push
```

If any commits violate the configured rules, the push will be rejected with details about the violations.
