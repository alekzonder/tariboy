ALTER TABLE agents ADD COLUMN goal_delivery_cooldown_s INTEGER NOT NULL DEFAULT 60 CHECK(goal_delivery_cooldown_s > 0);
ALTER TABLE agents ADD COLUMN last_goal_delivery_at TEXT NOT NULL DEFAULT '';
