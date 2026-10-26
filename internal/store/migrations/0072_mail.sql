-- 0072_mail.sql -- the per-principal mailbox (mailbox design §1). One row per
-- recipient; a letter is open while answered_at and dead_at are empty. The
-- bus tables stay until the cutover drops them.
CREATE TABLE mail (
    seq            INTEGER PRIMARY KEY AUTOINCREMENT, -- monotonic, for ingress
    id             TEXT NOT NULL UNIQUE,              -- m-<ts>-<n>, the public handle
    sender         TEXT NOT NULL,                     -- principal
    recipient      TEXT NOT NULL,                     -- principal
    subject        TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL DEFAULT 'message',   -- message | task.question | task.goal | ...
    data           TEXT NOT NULL DEFAULT '{}',        -- JSON, structured fields
    needs_reply    INTEGER NOT NULL DEFAULT 1,        -- 1 request, 0 notice
    in_reply_to    TEXT NOT NULL DEFAULT '',          -- id of the letter this answers
    deadline       TEXT NOT NULL DEFAULT '',          -- RFC3339; a request with a timeout
    idempotency_key TEXT,                             -- producer retry guard
    produced_in_iteration TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    read_at        TEXT NOT NULL DEFAULT '',
    answered_at    TEXT NOT NULL DEFAULT '',          -- closed: replied, archived or dead
    answer_id      TEXT NOT NULL DEFAULT '',          -- id of the reply, '' when archived
    close_note     TEXT NOT NULL DEFAULT '',          -- operator archive note, task comment ref
    attempts       INTEGER NOT NULL DEFAULT 0,        -- times rendered into a prompt
    dead_at        TEXT NOT NULL DEFAULT '',
    dead_reason    TEXT NOT NULL DEFAULT ''           -- attempts | queue_limit | decode_error
);
CREATE UNIQUE INDEX idx_mail_idempotency ON mail(idempotency_key, recipient) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_mail_inbox  ON mail(recipient, answered_at, created_at);
CREATE INDEX idx_mail_sent   ON mail(sender, created_at);
CREATE INDEX idx_mail_thread ON mail(in_reply_to);

-- A schedule addresses a principal instead of a channel. The service chat and
-- the old inbox of an agent become that agent; any other channel becomes the
-- agent that owns the schedule.
ALTER TABLE schedules RENAME COLUMN channel TO recipient;
UPDATE schedules SET recipient = CASE
    WHEN recipient GLOB 'chat:service:?*' AND instr(substr(recipient, 14), ':') = 0
        THEN 'agent:' || substr(recipient, 14)
    WHEN recipient GLOB 'agent:?*:inbox' AND instr(substr(recipient, 7, length(recipient) - 12), ':') = 0
        THEN 'agent:' || substr(recipient, 7, length(recipient) - 12)
    ELSE 'agent:' || agent
END;

-- Workflow ingress reads mail.seq, which starts again at 1: the cursor and
-- the trigger watermarks counted the old message sequence and start over.
UPDATE task_workflow_ingress_state SET last_message_sequence = 0;
UPDATE task_queue_workflow_triggers SET created_after_sequence = 0;

-- A plugin declares mail {send, receive} instead of channels; an installed
-- plugin's manifest is read again when it is installed or updated.
ALTER TABLE plugins RENAME COLUMN channels TO mail;
UPDATE plugins SET mail = '{}';
