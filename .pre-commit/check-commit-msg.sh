#!/usr/bin/env bash
# Validates commit messages against the Conventional Commits spec.
# https://www.conventionalcommits.org/en/v1.0.0/
#
# Called by the commit-msg pre-commit hook with the commit message file as $1.

set -e

commit_msg=$(cat "$1")

# Skip merge commits and revert auto-messages
if echo "$commit_msg" | grep -qE "^Merge |^Merged |^Revert \""; then
  exit 0
fi

# Skip fixup/squash commits (used during interactive rebase)
if echo "$commit_msg" | grep -qE "^(fixup|squash)!"; then
  exit 0
fi

# type(scope)!: description — scope and breaking-change marker are optional
pattern="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^)]+\))?(!)?: .+"
# Preferred scope: a JIRA ticket key, or deps/release for automation commits
jira_pattern="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)\(([A-Z]+-[0-9]+|deps|release)\)(!)?: .+"

if ! echo "$commit_msg" | grep -qE "$pattern"; then
  echo ""
  echo "  Commit message does not follow Conventional Commits spec."
  echo "  https://www.conventionalcommits.org/en/v1.0.0/"
  echo ""
  echo "  Format:  <type>(<scope>): <description>"
  echo "  Types:   feat | fix | docs | style | refactor | perf | test | build | ci | chore | revert"
  echo "  Scope:   preferably a JIRA ticket key (deps/release for automation commits)"
  echo "  Example: feat(EMIL-123): add OAuth2 login"
  echo "           fix(EMIL-456)!: drop Node 18 support (breaking change)"
  echo "           chore(deps): update tflint to v0.64.0"
  echo ""
  echo "  Got: $commit_msg"
  echo ""
  exit 1
fi

if ! echo "$commit_msg" | grep -qE "$jira_pattern"; then
  echo ""
  echo "  Warning: scope is not a JIRA ticket key (e.g. EMIL-123)."
  echo "  Preferred: <type>(<JIRA-ticket>): <description>"
  echo ""
  echo "  Got: $commit_msg"
  echo ""
fi
