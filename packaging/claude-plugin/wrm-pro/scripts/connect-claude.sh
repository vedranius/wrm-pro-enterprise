#!/bin/sh
# Connect Claude Code to WRM PRO over MCP.
#   ./connect-claude.sh https://wrm.example.com/mcp wrm_pat_xxx
# The token is passed as one argument to `claude mcp add`; it is never written
# to a file by this script.
set -eu

url="${1:-${WRM_URL:-}}"
token="${2:-${WRM_TOKEN:-}}"

if [ -z "$url" ] || [ -z "$token" ]; then
  printf 'usage: %s <wrm-mcp-url> <token>\n' "$0" >&2
  exit 2
fi

case "$url" in
  https://*) ;;
  http://127.0.0.1*|http://localhost*) printf 'local HTTP: using the token header (OAuth sign-in needs HTTPS)\n' ;;
  *) printf 'refusing a non-HTTPS address: %s\n' "$url" >&2; exit 2 ;;
esac

command -v claude >/dev/null 2>&1 || {
  printf 'Claude Code CLI not found; install it first: npm install -g @anthropic-ai/claude-code\n' >&2
  exit 127
}

claude mcp add --transport http --scope user wrm "$url" --header "Authorization: Bearer $token"
printf 'Added. Check it with: claude mcp list\n'
