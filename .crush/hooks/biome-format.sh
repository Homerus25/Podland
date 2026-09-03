#!/usr/bin/env bash
# Formats the target file with Biome before edit-type tools run, so agents
# always read and edit already-formatted content (no stale-read churn, no
# manual `biome check --write` runs). Non-formatting cases exit 0 silently.
set +e
file="$CRUSH_TOOL_INPUT_FILE_PATH"
[ -z "$file" ] && exit 0
[ -f "$file" ] || exit 0

case "$file" in
	*.ts|*.tsx|*.js|*.jsx|*.json|*.css) ;;
	*) exit 0 ;;
esac

# Biome config lives in frontend/; run from there so it is discovered.
root="$(git -C "$CRUSH_PROJECT_DIR" rev-parse --show-toplevel 2>/dev/null || echo "$CRUSH_PROJECT_DIR")"
cd "$root/frontend" 2>/dev/null || exit 0
command -v npx >/dev/null 2>&1 || exit 0
npx --no-install biome check --write --no-errors-on-unmatched "$file" >/dev/null 2>&1
exit 0
