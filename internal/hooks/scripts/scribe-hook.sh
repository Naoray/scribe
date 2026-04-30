#!/usr/bin/env bash
# scribe-hook.sh - Claude Code PostToolUseFailure hook.
#
# Stdin: Claude Code's PostToolUseFailure JSON payload.
# Stdout: JSON shaped as:
#   {"hookSpecificOutput":{"additionalContext":"..."}}
#
# The hook is fail-soft: it always emits valid JSON, even when scribe, jq, or
# individual scribe commands are unavailable.

set -u

# Detach from any inherited stdin / stdout-as-stdin. cat </dev/stdin would
# block whenever the pipe never EOFs (TTY invocation, half-closed pipe in
# tests). Closing stdin up-front is safer and ensures every child subshell
# inherits /dev/null rather than blocking on a read.
exec </dev/null

MAX_CONTEXT_CHARS=1800

escape_json_string() {
  if command -v jq >/dev/null 2>&1; then
    jq -Rn --arg value "$1" '$value'
    return
  fi

  local value=${1//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '"%s"' "$value"
}

emit_context() {
  local context=$1

  if [ "${#context}" -gt "$MAX_CONTEXT_CHARS" ]; then
    context="${context:0:$MAX_CONTEXT_CHARS}..."
  fi

  if command -v jq >/dev/null 2>&1; then
    jq -n --arg context "$context" '{
      hookSpecificOutput: {
        additionalContext: $context
      }
    }'
    return
  fi

  printf '{"hookSpecificOutput":{"additionalContext":%s}}\n' "$(escape_json_string "$context")"
}

json_length() {
  local payload=$1

  if ! command -v jq >/dev/null 2>&1; then
    printf ''
    return
  fi

  printf '%s' "$payload" | jq -r '
    if type == "array" then length
    elif type == "object" and (.skills | type) == "array" then .skills | length
    elif type == "object" and (.installed | type) == "array" then .installed | length
    elif type == "object" and (.items | type) == "array" then .items | length
    else empty
    end
  ' 2>/dev/null
}

status_summary() {
  local payload=$1

  if ! command -v jq >/dev/null 2>&1; then
    printf ''
    return
  fi

  printf '%s' "$payload" | jq -r '
    if type != "object" then empty
    else
      [
        (if (.installed_count | type) == "number" then "\(.installed_count) installed" else empty end),
        (if (.registries | type) == "array" then "\(.registries | length) registries" else empty end),
        (if (.last_sync | type) == "string" and .last_sync != "" then "last sync \(.last_sync)" else empty end)
      ] | join(", ")
    end
  ' 2>/dev/null
}

skill_names() {
  local payload=$1

  if ! command -v jq >/dev/null 2>&1; then
    printf ''
    return
  fi

  printf '%s' "$payload" | jq -r '
    def names:
      if type == "array" then .
      elif type == "object" and (.skills | type) == "array" then .skills
      elif type == "object" and (.installed | type) == "array" then .installed
      elif type == "object" and (.items | type) == "array" then .items
      else []
      end
      | map(if type == "object" then .name // empty else empty end)
      | map(select(. != ""));
    names | .[:8] | join(", ")
  ' 2>/dev/null
}

if ! command -v scribe >/dev/null 2>&1; then
  emit_context "scribe not available. Suggested next steps: install scribe or run scribe doctor once scribe is on PATH."
  exit 0
fi

context="Scribe is available on PATH."

list_json=$(scribe list --json 2>/dev/null) || list_json=''
if [ -n "$list_json" ]; then
  installed_count=$(json_length "$list_json")
  names=$(skill_names "$list_json")
  if [ -n "$installed_count" ]; then
    context="$context Installed skills: $installed_count."
  else
    context="$context Installed skills: available via 'scribe list --json'."
  fi
  if [ -n "$names" ]; then
    context="$context Examples: $names."
  fi
else
  context="$context Installed skills: unavailable right now."
fi

status_json=$(scribe status --json 2>/dev/null) || status_json=''
if [ -n "$status_json" ]; then
  summary=$(status_summary "$status_json")
  if [ -n "$summary" ]; then
    context="$context Registry status: $summary."
  else
    context="$context Registry status: available via 'scribe status --json'."
  fi
else
  context="$context Registry status: unavailable right now."
fi

context="$context Suggested commands: scribe list, scribe status, scribe explain <name>, scribe install <name>, scribe doctor."

emit_context "$context"
