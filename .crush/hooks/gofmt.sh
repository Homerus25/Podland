#!/usr/bin/env bash
# Formats Go files with gofmt before edit-type tools run, mirroring the
# biome hook for frontend files: agents always read and edit
# already-formatted content, and stray indentation can't slip through.
# Non-Go targets exit 0 silently.
set +e
file="$CRUSH_TOOL_INPUT_FILE_PATH"
[ -z "$file" ] && exit 0
case "$file" in
	*.go) ;;
	*) exit 0 ;;
esac
[ -f "$file" ] || exit 0
command -v gofmt >/dev/null 2>&1 || exit 0
gofmt -w "$file" >/dev/null 2>&1
exit 0
