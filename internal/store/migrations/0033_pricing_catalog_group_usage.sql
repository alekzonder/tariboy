ALTER TABLE ai_pricing RENAME TO ai_pricing_legacy;
CREATE TABLE ai_pricing (
  model TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'manual' CHECK(source IN ('manual','litellm')),
  input_per_mtok REAL NOT NULL DEFAULT 0,
  output_per_mtok REAL NOT NULL DEFAULT 0,
  cache_write_per_mtok REAL NOT NULL DEFAULT 0,
  cache_read_per_mtok REAL NOT NULL DEFAULT 0,
  PRIMARY KEY(model, source)
);
INSERT INTO ai_pricing(model,source,input_per_mtok,output_per_mtok,cache_write_per_mtok,cache_read_per_mtok)
SELECT model,'manual',input_per_mtok,output_per_mtok,cache_write_per_mtok,cache_read_per_mtok
FROM ai_pricing_legacy
WHERE NOT (
  (model='claude-opus-4-8' AND input_per_mtok=5 AND output_per_mtok=25 AND cache_write_per_mtok=6.25 AND cache_read_per_mtok=0.5) OR
  (model='claude-sonnet-4-6' AND input_per_mtok=3 AND output_per_mtok=15 AND cache_write_per_mtok=3.75 AND cache_read_per_mtok=0.3) OR
  (model='claude-haiku-4-5' AND input_per_mtok=1 AND output_per_mtok=5 AND cache_write_per_mtok=1.25 AND cache_read_per_mtok=0.1) OR
  (model='gpt-4o' AND input_per_mtok=2.5 AND output_per_mtok=10 AND cache_write_per_mtok=0 AND cache_read_per_mtok=1.25)
);
DROP TABLE ai_pricing_legacy;
ALTER TABLE ai_requests ADD COLUMN group_id TEXT;
ALTER TABLE ai_requests ADD COLUMN group_name TEXT;
CREATE INDEX idx_ai_requests_group_ts ON ai_requests(group_id, ts);
