#!/usr/bin/env bash
# Stub harness for tests and E2E. Not a real harness: it reads the prompt path
# passed as $1, optionally signals completion via the agent bin PATH, and exits.
# Behaviour env vars (all optional):
#   STUB_SLEEP     seconds to sleep before finishing        (default 0.2)
#   STUB_CALL_DONE 1 to call i-am-done, 0 to skip           (default 1)
#   STUB_EXIT      exit code                                (default 0)
#   STUB_SEND      "to|text" to mail a request before finishing (default unset)
#   STUB_SCHEDULE  seconds ahead to arm a one-shot (once)     (default unset)
#   STUB_GROUP_REQUEST "member|text|deadline" to mail a request with a deadline (once)
#   STUB_REPLY_INBOX "text" to reply to the first open letter, then drain requests
#   STUB_IMAGE_BUILD "name:tag|path" to author+build an image via the gated tool (default unset)
#   STUB_AI  set to 1 to POST one AI request through the proxy before finishing (default unset)
#   STUB_STDOUT    line to print before finishing             (default unset)
#   STUB_TASKS_MINE file to receive a direct ttasks mine result (default unset)
set -eu

tool_post() {
  curl -sf --unix-socket "$TARIBOY_TOOLS_SOCKET" -H 'content-type: application/json' \
    -X POST "http://localhost$1" -d "$2"
}

if [ "${1:-}" = "--version" ]; then
  printf '%s\n' '2.1.227'
  exit 0
fi

PROMPT_PATH="${1:-}"
[ -n "$PROMPT_PATH" ] && [ -f "$PROMPT_PATH" ] && head -c 0 "$PROMPT_PATH" || true

if [ -n "${STUB_TASKS_MINE:-}" ]; then
  ttasks mine --json >"$STUB_TASKS_MINE"
fi

sleep "${STUB_SLEEP:-0.2}"

if [ -n "${STUB_STDOUT:-}" ]; then
  printf '%s\n' "$STUB_STDOUT"
fi

# Optional: mail a letter (a request) before finishing. The recipient is a
# principal such as agent:bob or group:team.
#   STUB_SEND="agent:bob|hello from A"
if [ -n "${STUB_SEND:-}" ]; then
  SEND_TO="${STUB_SEND%%|*}"
  SEND_TEXT="${STUB_SEND#*|}"
  tool_post /tools/mail/send "{\"to\":\"$SEND_TO\",\"subject\":\"$SEND_TEXT\",\"text\":\"$SEND_TEXT\"}" >/dev/null 2>&1 || true
fi

# Optional: arm a one-shot schedule N seconds ahead, exactly once (a marker in
# the agent cwd prevents re-arming on the message-triggered iteration it fires).
#   STUB_SCHEDULE="2"
if [ -n "${STUB_SCHEDULE:-}" ] && [ ! -f .stub_scheduled ]; then
  SPEC="$(date -u -d "+${STUB_SCHEDULE} seconds" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)"
  if [ -n "$SPEC" ]; then
    tool_post /tools/schedule/add "{\"kind\":\"oneshot\",\"spec\":\"$SPEC\"}" >/dev/null 2>&1 || true
    : > .stub_scheduled
  fi
fi

# Optional: mail a request with a deadline to a teammate, exactly once (a
# marker in the agent cwd prevents re-sending on later iterations). The deadline
# arms a one-shot schedule; with no reply the sender gets a kind=deadline notice
# from system:schedule.
#   STUB_GROUP_REQUEST="member|text|deadline"   e.g. "worker|blocking?|3s"
if [ -n "${STUB_GROUP_REQUEST:-}" ] && [ ! -f .stub_group_req ]; then
  GR_MEMBER="${STUB_GROUP_REQUEST%%|*}"
  GR_REST="${STUB_GROUP_REQUEST#*|}"
  GR_TEXT="${GR_REST%%|*}"
  GR_DEADLINE="${GR_REST#*|}"
  tool_post /tools/mail/send "{\"to\":\"agent:$GR_MEMBER\",\"subject\":\"$GR_TEXT\",\"text\":\"$GR_TEXT\",\"deadline\":\"$GR_DEADLINE\"}" >/dev/null 2>&1 || true
  : > .stub_group_req
fi

# Optional: author+build a new image via the gated image-build tool.
#   STUB_IMAGE_BUILD="name:tag|relative-path"
#   STUB_IMAGE_BUILD_OUT=/abs/file  (optional: capture the tool output)
if [ -n "${STUB_IMAGE_BUILD:-}" ]; then
  IB_TAG="${STUB_IMAGE_BUILD%%|*}"
  IB_PATH="${STUB_IMAGE_BUILD#*|}"
  tool_post /tools/image/build "{\"name\":\"${IB_TAG%%:*}\",\"tag\":\"${IB_TAG#*:}\",\"path\":\"$IB_PATH\"}" >"${STUB_IMAGE_BUILD_OUT:-/dev/null}" 2>&1 || true
fi

# Optional: drive a real AI call through the proxy before finishing.
#   STUB_AI=1   (uses ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN injected by the runner)
if [ -n "${STUB_AI:-}" ]; then
  # Attribution rides the tokenized ANTHROPIC_BASE_URL path (/_tariboy/<token>),
  # so the x-api-key header is not the attribution channel. ANTHROPIC_AUTH_TOKEN is
  # the provider key, which is unset in tokenless test/e2e runs — default it to empty
  # so `set -u` does not abort the harness before the call is made.
  curl -s -X POST "${ANTHROPIC_BASE_URL}/v1/messages" \
    -H "x-api-key: ${ANTHROPIC_AUTH_TOKEN:-}" \
    -H "content-type: application/json" \
    -d '{"model":"claude-opus-4-8","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}' \
    >/dev/null 2>&1 || true
fi

# Optional: dump this iteration's rendered prompt to a file so an e2e can assert
# on the inline Messages section (subject/data.files a source plugin published).
# APPENDS (with a separator) rather than overwrites: an agent runs extra
# message-triggered iterations while letters stay open, and a plain overwrite
# would leave only the last one's prompt. Accumulating keeps every iteration's
# Messages section greppable.
#   STUB_DUMP_PROMPT=/abs/file
if [ -n "${STUB_DUMP_PROMPT:-}" ] && [ -n "$PROMPT_PATH" ] && [ -f "$PROMPT_PATH" ]; then
  { printf '\n===== iteration prompt =====\n'; cat "$PROMPT_PATH"; } >> "$STUB_DUMP_PROMPT" 2>/dev/null || true
fi

# Optional: reply to the FIRST open letter in this agent's inbox, exactly once
# (a marker in the agent cwd guards it), then drain by replying to every other
# open request so the loop goes quiet instead of re-prompting forever. Open
# notices close on their own when this iteration finishes successfully.
#   STUB_REPLY_INBOX="reply text"
#   STUB_REPLY_INBOX_OUT=/abs/file  (optional: capture the first reply result)
if [ -n "${STUB_REPLY_INBOX:-}" ]; then
  INBOX="$(tool_post /tools/mail/inbox '{}' 2>/dev/null || true)"
  FIRST_ID="$(printf '%s' "$INBOX" | jq -r '.letters[0].id // empty' 2>/dev/null || true)"
  if [ -n "$FIRST_ID" ] && [ ! -f .stub_replied ]; then
    tool_post /tools/mail/reply "{\"id\":\"$FIRST_ID\",\"text\":\"$STUB_REPLY_INBOX\"}" >"${STUB_REPLY_INBOX_OUT:-/dev/null}" 2>&1 || true
    : > .stub_replied
  fi
  for MID in $(printf '%s' "$INBOX" | jq -r '.letters[] | select(.needs_reply) | .id' 2>/dev/null || true); do
    [ "$MID" = "$FIRST_ID" ] && continue
    tool_post /tools/mail/reply "{\"id\":\"$MID\",\"text\":\"e2e-drain\"}" >/dev/null 2>&1 || true
  done
fi

if [ "${STUB_CALL_DONE:-1}" = "1" ]; then
  tool_post /tools/loop/done '{"idle":false}' >/dev/null 2>&1 || true
fi

exit "${STUB_EXIT:-0}"
