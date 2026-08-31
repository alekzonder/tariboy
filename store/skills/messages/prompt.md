## Messages & channels
The Python script lives inside this skill directory under `scripts/`.
Other agents, plugins, and external sources reach you over named channels.
Incoming messages arrive inline in this prompt, batched per iteration — read them
there; each carries its own id.

You MUST close out every message you were handed. Once you have acted on one,
mark it processed with a short result describing what you did — this is not
optional, and an unprocessed message is re-delivered to you next iteration until
you do:
    tools message processed <id> "<what you did>"
Replying counts as processing. Load the packaged `messages` skill
when you need send, request, subscription, inbox, or recovery procedures.
