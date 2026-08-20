## Messages & channels
Other agents, plugins, and external sources reach you over named channels.
Incoming messages arrive inline in this prompt, batched per iteration — read them
there; each carries its own id.

You MUST close out every message you were handed. Once you have acted on one,
mark it processed with a short result describing what you did — this is not
optional, and an unprocessed message is re-delivered to you next iteration until
you do:
    tools message processed <id> "<what you did>"
Replying counts as processing: a reply auto-processes the message it answers.
    tools message reply <id> --text <body> [--data <json>]

To publish — notify a teammate, hand off work, broadcast — run:
    tools message send --channel <name> --text <body>
Attach structured fields with --type, --subject k=v,.. or --data <json> when a
consumer expects them.

Ask another agent or a provider for something and wait for the answer with a
request; with --deadline you get a timeout event in your inbox if no reply lands:
    tools request --channel <name> --text <body> [--deadline 5m]

Inspect your own inbox and recover dead-lettered messages (dropped after too many
un-processed deliveries):
    tools message ls [--all]             # pending inbox; --all adds archive + dlq
    tools message dlq                    # dead-lettered messages
    tools message dlq requeue <id>       # send one back to the pending queue

Control what reaches you:
    tools sources                        # channels you can subscribe to
    tools channel subscribe <name> [--type <globs>] [--matcher <json>] [--params <json>]
    tools channel ls                     # your subscriptions, incl. params/watch
    tools channel unsubscribe <sub-id>   # by id (or a channel name to drop all its subs)
Rule of thumb for what to pass: if the data already flows on the channel, narrow
it with --matcher (and --type); if a provider has to do work to produce it for
you, describe that work in --params. The daemon's channel bus delivers what you
send and routes subscribed channels into your next iteration.
