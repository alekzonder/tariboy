#!/usr/bin/env bash
# Stub harness for tests and E2E. Not a real harness: it reads the prompt path
# passed as $1, optionally signals completion via the agent bin PATH, and exits.
# Behaviour env vars (all optional):
#   STUB_SLEEP     seconds to sleep before finishing        (default 0.2)
#   STUB_CALL_DONE 1 to call i-am-done, 0 to skip           (default 1)
#   STUB_EXIT      exit code                                (default 0)
#   STUB_SUBSCRIBE channel to subscribe to before finishing  (default unset)
#   STUB_SEND      "channel|text" to publish before finishing (default unset)
#   STUB_SCHEDULE  seconds ahead to arm a one-shot (once)     (default unset)
#   STUB_IMAGE_BUILD "name:tag|path" to author+build an image via the gated tool (default unset)
#   STUB_AI  set to 1 to POST one AI request through the proxy before finishing (default unset)
#   STUB_STDOUT    line to print before finishing             (default unset)
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MESSAGES="$ROOT/store/skills/messages/scripts/messages.sh"
SCHEDULE="$ROOT/store/skills/schedule/scripts/schedule.sh"
IMAGE_CREATOR="$ROOT/store/skills/image-creator/scripts/image_creator.sh"

if [ "${1:-}" = "--version" ]; then
  printf '%s\n' '2.1.227'
  exit 0
fi

PROMPT_PATH="${1:-}"
[ -n "$PROMPT_PATH" ] && [ -f "$PROMPT_PATH" ] && head -c 0 "$PROMPT_PATH" || true

sleep "${STUB_SLEEP:-0.2}"

if [ -n "${STUB_STDOUT:-}" ]; then
  printf '%s\n' "$STUB_STDOUT"
fi

# Optional: subscribe this agent to a channel before finishing.
#   STUB_SUBSCRIBE="chat:room"
if [ -n "${STUB_SUBSCRIBE:-}" ]; then
  "$MESSAGES" channel subscribe "$STUB_SUBSCRIBE" >/dev/null 2>&1 || true
fi

# Optional: emit a message before finishing.
#   STUB_SEND="chat:room|hello from A"
if [ -n "${STUB_SEND:-}" ]; then
  SEND_CHANNEL="${STUB_SEND%%|*}"
  SEND_TEXT="${STUB_SEND#*|}"
  "$MESSAGES" message send --channel "$SEND_CHANNEL" --text "$SEND_TEXT" >/dev/null 2>&1 || true
fi

# Optional: arm a one-shot schedule N seconds ahead, exactly once (a marker in
# the agent cwd prevents re-arming on the message-triggered iteration it fires).
#   STUB_SCHEDULE="2"
if [ -n "${STUB_SCHEDULE:-}" ] && [ ! -f .stub_scheduled ]; then
  SPEC="$(date -u -d "+${STUB_SCHEDULE} seconds" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)"
  if [ -n "$SPEC" ]; then
    "$SCHEDULE" add --kind oneshot --spec "$SPEC" >/dev/null 2>&1 || true
    : > .stub_scheduled
  fi
fi

# Optional: send a group request to a teammate, exactly once (a marker in the
# agent cwd prevents re-sending on later iterations). Drives the request
# primitive (spec §4.2) end to end: kind=request onto the member's group direct
# channel with an armed --deadline.
#   STUB_GROUP_REQUEST="member|text|deadline"   e.g. "worker|blocking?|3s"
if [ -n "${STUB_GROUP_REQUEST:-}" ] && [ ! -f .stub_group_req ]; then
  GR_MEMBER="${STUB_GROUP_REQUEST%%|*}"
  GR_REST="${STUB_GROUP_REQUEST#*|}"
  GR_TEXT="${GR_REST%%|*}"
  GR_DEADLINE="${GR_REST#*|}"
  "$MESSAGES" group request "$GR_MEMBER" --text "$GR_TEXT" --deadline "$GR_DEADLINE" >/dev/null 2>&1 || true
  : > .stub_group_req
fi

# Optional: author+build a new image via the gated image-build tool.
#   STUB_IMAGE_BUILD="name:tag|relative-path"
#   STUB_IMAGE_BUILD_OUT=/abs/file  (optional: capture the tool output)
if [ -n "${STUB_IMAGE_BUILD:-}" ]; then
  IB_TAG="${STUB_IMAGE_BUILD%%|*}"
  IB_PATH="${STUB_IMAGE_BUILD#*|}"
  "$IMAGE_CREATOR" build --name "${IB_TAG%%:*}" --tag "${IB_TAG#*:}" --path "$IB_PATH" >"${STUB_IMAGE_BUILD_OUT:-/dev/null}" 2>&1 || true
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
# APPENDS (with a separator) rather than overwrites: an agent subscribed to a
# channel it also replies on runs extra message-triggered iterations, and a plain
# overwrite would leave only the last one's prompt. Accumulating keeps every
# iteration's Messages section greppable.
#   STUB_DUMP_PROMPT=/abs/file
if [ -n "${STUB_DUMP_PROMPT:-}" ] && [ -n "$PROMPT_PATH" ] && [ -f "$PROMPT_PATH" ]; then
  { printf '\n===== iteration prompt =====\n'; cat "$PROMPT_PATH"; } >> "$STUB_DUMP_PROMPT" 2>/dev/null || true
fi

# Optional: reply to the FIRST inbox message rendered into the prompt, then drain
# the rest. The Messages section lists each pending message as
# "- id <ID> [type] source: text"; replying kind=reply auto-processes the target
# and (for a message a sink plugin can route) threads the outbound against the
# original — driving the source->agent->sink round-trip. The reply fires exactly
# once (a marker in the agent cwd guards it); every remaining pending message is
# then processed so an agent that receives its own reply back (it is subscribed to
# the channel it replied on) drains its inbox and the loop goes quiet instead of
# re-prompting forever.
#   STUB_REPLY_INBOX="reply text"
if [ -n "${STUB_REPLY_INBOX:-}" ] && [ -n "$PROMPT_PATH" ] && [ -f "$PROMPT_PATH" ]; then
  IDS="$(sed -n 's/^- id \([^ ]*\).*/\1/p' "$PROMPT_PATH")"
  if [ -n "$IDS" ] && [ ! -f .stub_replied ]; then
    FIRST_ID="$(printf '%s\n' "$IDS" | head -n1)"
    "$MESSAGES" message reply "$FIRST_ID" --text "$STUB_REPLY_INBOX" >"${STUB_REPLY_INBOX_OUT:-/dev/null}" 2>&1 || true
    : > .stub_replied
  fi
  # Drain any still-pending messages (the just-replied one auto-processed; a
  # no-op re-process is harmless).
  for MID in $IDS; do
    "$MESSAGES" message processed "$MID" "e2e-drain" >/dev/null 2>&1 || true
  done
fi

if [ "${STUB_CALL_DONE:-1}" = "1" ]; then
  # bin/i-am-done is the one PATH compatibility shim.
  i-am-done >/dev/null 2>&1 || true
fi

exit "${STUB_EXIT:-0}"
