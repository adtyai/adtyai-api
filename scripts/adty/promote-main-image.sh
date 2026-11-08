#!/usr/bin/env bash
set -euo pipefail

# The workflow serializes this promotion job, independently of image builds.
# Resolve main inside that lock so a slow old build cannot roll back :main.
: "${GITHUB_REPOSITORY:?}"
: "${GITHUB_SHA:?}"
digest=${1:?image digest required}
if [[ ! $digest =~ ^sha256:[0-9a-f]{64}$ || ! $GITHUB_SHA =~ ^[0-9a-f]{40}$ ]]; then
  echo 'Invalid image digest or commit SHA' >&2
  exit 1
fi
tip=$(gh api "repos/${GITHUB_REPOSITORY}/commits/main" --jq .sha)
if [[ $tip != "$GITHUB_SHA" ]]; then
  echo "main is now at $tip; skip promotion for $GITHUB_SHA"
  exit 0
fi
image="ghcr.io/${GITHUB_REPOSITORY,,}"
docker buildx imagetools create --tag "$image:main" "$image@$digest"
