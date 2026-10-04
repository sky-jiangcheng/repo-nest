#!/usr/bin/env bash
# Bump the project version across all locations, keeping them in sync.
#
# SSOT (single source of truth): wails.json -> info.productVersion
# This script writes the new version into wails.json, then propagates it to:
#   - web/package.json           (version field)
#   - internal/version/version.go                     (const Version, also overridable via ldflags at CI build time)
#
# Usage:
#   ./scripts/bump-version.sh 1.5.4
#   ./scripts/bump-version.sh   # no arg = read current version from wails.json, just sync
#
# After running: git commit, git tag v<X.Y.Z>, then push that one tag.
# Push the version tag explicitly rather than `git push --tags`: the latter
# also pushes every local tag and fails outright when any historical tag
# disagrees with the remote, which can leave the intended release unpushed.
set -euo pipefail

# Accept either "1.5.4" or "v1.5.4"; store without leading v.
RAW_VERSION="${1:-}"
if [[ -n "$RAW_VERSION" ]]; then
  VERSION="${RAW_VERSION#v}"
else
  # No arg: just re-sync from wails.json (idempotent).
  VERSION="$(grep -oE '"productVersion"[[:space:]]*:[[:space:]]*"[^"]+"' wails.json | head -1 | sed -E 's/.*"([^"]+)"$/\1/')"
  if [[ -z "$VERSION" ]]; then
    echo "ERROR: could not read productVersion from wails.json" >&2
    exit 1
  fi
  echo "No version given; re-syncing from wails.json -> $VERSION"
fi

# Validate semver-ish.
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([+-][A-Za-z0-9.-]+)?$ ]]; then
  echo "ERROR: '$VERSION' is not a valid X.Y.Z version" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

update_file() {
  local file="$1" pattern="$2" replacement="$3"
  if [[ ! -f "$file" ]]; then
    echo "  WARN: $file not found, skipping" >&2
    return 0
  fi
  # Portable in-place edit via a temp file. `sed -i` is not portable: BSD
  # requires an argument (sed -i ''), GNU accepts none, and toybox sed (often
  # first on PATH inside tool shims) silently treats the next token as a script.
  local tmp
  tmp="$(mktemp "${file}.XXXXXX")"
  if ! sed -E "s|$pattern|$replacement|" "$file" > "$tmp"; then
    echo "  ERROR: sed failed for $file" >&2
    rm -f "$tmp"
    return 1
  fi
  mv "$tmp" "$file"
  echo "  updated $file"
}

echo "Bumping version to $VERSION"

# NOTE: the quotes are captured on the replacement side, not the pattern side.
# Writing "\1$VERSION" would expand to e.g. "\11.7.6", which sed reads as
# back-reference 11 (missing) instead of group 1 followed by the literal "1.7.6".

# 1. wails.json  ->  info.productVersion  (SSOT, set first)
update_file "wails.json" \
  '("productVersion"[[:space:]]*:[[:space:]]*)"[^"]+"' \
  "\1\"$VERSION\""

# 2. web/package.json  ->  version
update_file "web/package.json" \
  '("version"[[:space:]]*:[[:space:]]*)"[^"]+"' \
  "\1\"$VERSION\""

# 3. internal/version/version.go  ->  const Version = "..."  (SSOT for app/CLI/MCP)
update_file "internal/version/version.go" \
  '(const Version[[:space:]]*=[[:space:]]*)"[^"]+"' \
  "\1\"$VERSION\""

# 3. web/package-lock.json  ->  the two root "version" fields.
#
#    npm keeps these in sync with package.json, but a version bump does not run
#    an install, so they go stale. The trap: the root entry (packages[""]) and
#    every dependency entry are BOTH indented six spaces, so indentation cannot
#    tell them apart. What does distinguish them is that the root version is
#    the only one in the file carrying the old version string alongside the root
#    "name" - so we assert the count before rewriting, and refuse to touch the
#    file if the assumption is wrong rather than silently stamping 380
#    dependency entries with the project version.
if [ -f web/package-lock.json ]; then
  # Read the lockfile's own version - package.json above has already been
  # rewritten, so it can no longer tell us what the lockfile currently holds.
  LOCK_CUR="$(grep -m1 -oE '"version"[[:space:]]*:[[:space:]]*"[^"]+"' web/package-lock.json | sed -E 's/.*"([^"]+)"$/\1/')"
  if [ -n "$LOCK_CUR" ] && [ "$LOCK_CUR" != "$VERSION" ]; then
    HITS=$(grep -c "\"version\"[[:space:]]*:[[:space:]]*\"$LOCK_CUR\"" web/package-lock.json || true)
    if [ "$HITS" -ne 2 ]; then
      echo "ERROR: web/package-lock.json has $HITS occurrences of version $LOCK_CUR, expected 2." >&2
      echo "       Refusing to rewrite it — a dependency may share that version." >&2
      echo "       Update the lockfile with 'cd web && npm install --package-lock-only'." >&2
      exit 1
    fi
    update_file "web/package-lock.json" \
      "(\"version\"[[:space:]]*:[[:space:]]*)\"$LOCK_CUR\"" \
      "\1\"$VERSION\""
  fi
fi

# 4. packaging manifests (Homebrew Cask / Formula, Scoop) — delegated so the
#    Ruby + JSON rewriting rules live in exactly one place.
./scripts/update-manifests.sh

# 5. docs 站版本徽章随生成脚本读取 web/package.json，无需手工更新：
#    node scripts/build-docs.mjs

echo
echo "Done. Verify with:"
echo "  grep -rn '$VERSION' wails.json web/package.json web/package-lock.json internal/version/version.go packaging/ docs/index.html"
echo "  go build ./... && (cd web && npm run build)"
echo
echo "Then commit & tag:"
echo "  git add -A && git commit -m \"chore: bump version to $VERSION\""
echo "  git tag -a v$VERSION -m \"Release v$VERSION\""
echo "  git push github master && git push github v<X.Y.Z>"
