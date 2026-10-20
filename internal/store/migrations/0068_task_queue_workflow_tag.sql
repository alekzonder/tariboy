-- The tag a queue binding was made by. A binding with a tag follows it: when
-- the tag moves, the binding moves to the new digest. An empty tag pins the
-- digest; every earlier binding stays pinned, because its tag was not kept.
ALTER TABLE task_queue_workflows ADD COLUMN workflow_tag TEXT NOT NULL DEFAULT '';
