# Connect Claude Code to WRM PRO
#   .\connect-claude.ps1 -Url https://wrm.example.com/mcp -Token wrm_pat_xxx
# The token is passed as one argument to `claude mcp add`; this script never
# writes it to a file.
param(
  [string]$Url = $env:WRM_URL,
  [string]$Token = $env:WRM_TOKEN
)

$ErrorActionPreference = 'Stop'
if (-not $Url -or -not $Token) {
  Write-Error "usage: .\connect-claude.ps1 -Url <wrm-mcp-url> -Token <token>"
  exit 2
}
if (-not ($Url -like 'https://*' -or $Url -like 'http://127.0.0.1*' -or $Url -like 'http://localhost*')) {
  Write-Error "refusing a non-HTTPS address: $Url"
  exit 2
}
if ($Url -like 'http://*') {
  Write-Host 'local HTTP: using the token header (OAuth sign-in needs HTTPS)'
}
if (-not (Get-Command claude -ErrorAction SilentlyContinue)) {
  Write-Error 'Claude Code CLI not found; install it first: npm install -g @anthropic-ai/claude-code'
  exit 127
}

& claude mcp add --transport http --scope user wrm $Url --header "Authorization: Bearer $Token"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host 'Added. Check it with: claude mcp list'
