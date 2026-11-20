#!/bin/bash
# Report what Claude Code is doing to Marquee (http://127.0.0.1:7781), for the claudelive screen
# and the activity line under each claudehive orb. Install as hooks (see docs/GUIDE.md):
#
#   claude-hook.sh tool     PreToolUse        a tool is starting
#   claude-hook.sh done     PostToolUse       it finished
#   claude-hook.sh prompt   UserPromptSubmit  you sent a message (its text is not sent)
#   claude-hook.sh stop     Stop              Claude finished its turn
#
# Sends only the session id, the project folder, the tool name and a short detail (a file's
# name, a command's description, a search pattern) to this Mac. Silent and quick: if Marquee
# is not running nothing happens. Needs jq and curl.

mode="${1:-}"
case "$mode" in tool | done | prompt | stop) ;; *) exit 0 ;; esac
command -v jq >/dev/null 2>&1 || exit 0
url="${MARQUEE_URL:-http://127.0.0.1:7781}/v1/claude/activity"

body="$(jq -c --arg e "$mode" '
  def base: tostring | split("/") | last;
  (.tool_input // {}) as $i
  | (if ($i.file_path // $i.notebook_path) then (($i.file_path // $i.notebook_path) | base)
     elif $i.description then $i.description
     elif $i.command then ($i.command | tostring | .[0:80])
     elif $i.pattern then $i.pattern
     elif $i.url then $i.url
     elif $i.query then $i.query
     elif $i.skill then $i.skill
     else "" end) as $d
  | {event: $e, session: (.session_id // ""), cwd: (.cwd // ""), tool: (.tool_name // ""),
     detail: ($d | tostring | .[0:120])}' 2>/dev/null)" || exit 0
[ -n "$body" ] || exit 0
curl -s -m 2 -o /dev/null -X POST -H 'Content-Type: application/json' -d "$body" "$url" >/dev/null 2>&1
exit 0
