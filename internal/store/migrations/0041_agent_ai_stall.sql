ALTER TABLE agents ADD COLUMN ai_stall_timeout_s INTEGER NOT NULL DEFAULT 300 CHECK(ai_stall_timeout_s > 0);
ALTER TABLE iterations ADD COLUMN last_ai_request_at TEXT NOT NULL DEFAULT '';
