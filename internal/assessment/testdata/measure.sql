-- Measure the latest-assessment selection on a daemon database, read-only:
--   sqlite3 -readonly "$TARIBOY_BASE_DIR/tariboyd.db" < measure.sql
-- An object is an iteration (iteration_id) or, with an empty iteration_id, a
-- task (task_key). The SUM condition is assessment.IsLatest verbatim, the
-- condition metrics and assessment ls use; TestMeasureQuery fails when they
-- differ. The object column groups records without it.
--   objects_with_several_assessments  objects with more than one record
--   rows_of_those_objects             their records, all summed before the fix
--   latest_rows_of_those_objects      their records metrics take now; equals
--                                     objects_with_several_assessments
--   objects_with_several_latest_rows  objects with more than one latest
--                                     record; expected 0
WITH objects AS (
  SELECT CASE WHEN a.iteration_id <> '' THEN 'iteration:' || a.iteration_id ELSE 'task:' || a.task_key END AS object,
    COUNT(*) AS total,
    SUM(
CASE WHEN a.iteration_id <> ''
	THEN NOT EXISTS (SELECT 1 FROM assessments b WHERE b.iteration_id = a.iteration_id AND (b.created_at > a.created_at OR (b.created_at = a.created_at
		AND ((CASE WHEN rtrim(b.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(b.rubric_version, '0123456789')) < length(b.rubric_version) THEN substr(rtrim(b.rubric_version, '0123456789'), 1, length(rtrim(b.rubric_version, '0123456789')) - 1) ELSE b.rubric_version END), (CASE WHEN rtrim(b.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(b.rubric_version, '0123456789')) < length(b.rubric_version) THEN CAST(substr(b.rubric_version, length(rtrim(b.rubric_version, '0123456789')) + 1) AS INTEGER) ELSE -1 END), b.seq) > ((CASE WHEN rtrim(a.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(a.rubric_version, '0123456789')) < length(a.rubric_version) THEN substr(rtrim(a.rubric_version, '0123456789'), 1, length(rtrim(a.rubric_version, '0123456789')) - 1) ELSE a.rubric_version END), (CASE WHEN rtrim(a.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(a.rubric_version, '0123456789')) < length(a.rubric_version) THEN CAST(substr(a.rubric_version, length(rtrim(a.rubric_version, '0123456789')) + 1) AS INTEGER) ELSE -1 END), a.seq))))
	ELSE NOT EXISTS (SELECT 1 FROM assessments b WHERE b.iteration_id = '' AND b.task_key = a.task_key AND (b.created_at > a.created_at OR (b.created_at = a.created_at
		AND ((CASE WHEN rtrim(b.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(b.rubric_version, '0123456789')) < length(b.rubric_version) THEN substr(rtrim(b.rubric_version, '0123456789'), 1, length(rtrim(b.rubric_version, '0123456789')) - 1) ELSE b.rubric_version END), (CASE WHEN rtrim(b.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(b.rubric_version, '0123456789')) < length(b.rubric_version) THEN CAST(substr(b.rubric_version, length(rtrim(b.rubric_version, '0123456789')) + 1) AS INTEGER) ELSE -1 END), b.seq) > ((CASE WHEN rtrim(a.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(a.rubric_version, '0123456789')) < length(a.rubric_version) THEN substr(rtrim(a.rubric_version, '0123456789'), 1, length(rtrim(a.rubric_version, '0123456789')) - 1) ELSE a.rubric_version END), (CASE WHEN rtrim(a.rubric_version, '0123456789') LIKE '_%/' AND length(rtrim(a.rubric_version, '0123456789')) < length(a.rubric_version) THEN CAST(substr(a.rubric_version, length(rtrim(a.rubric_version, '0123456789')) + 1) AS INTEGER) ELSE -1 END), a.seq)))) END
    ) AS taken
  FROM assessments a
  GROUP BY object
)
SELECT
  (SELECT COUNT(*) FROM objects WHERE total > 1) AS objects_with_several_assessments,
  (SELECT COALESCE(SUM(total), 0) FROM objects WHERE total > 1) AS rows_of_those_objects,
  (SELECT COALESCE(SUM(taken), 0) FROM objects WHERE total > 1) AS latest_rows_of_those_objects,
  (SELECT COUNT(*) FROM objects WHERE taken > 1) AS objects_with_several_latest_rows;
