#!/usr/bin/env bash
# A45 legacy defaults importer wrapper.
# Default: dry-run only. Do NOT source _accept_env.sh (avoids locking / writing联调 PG).
# For -apply / -rollback: export SQL_DSN yourself after I5; never run against prod without approval.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

MODE="dry-run"
ARGS=()
for a in "$@"; do
  case "$a" in
    -apply|-rollback) MODE="$a" ;;
  esac
  ARGS+=("$a")
done

if [[ "$MODE" != "dry-run" && -z "${SQL_DSN:-}" ]]; then
  echo "run_import_a45_legacy_defaults: $MODE requires SQL_DSN; refusing to load联调 secrets automatically." >&2
  exit 1
fi

echo "A45 import mode=$MODE (default dry-run is safe)"
exec go run ./scripts/import_a45_legacy_defaults.go -repo "$REPO_ROOT" "${ARGS[@]}"
