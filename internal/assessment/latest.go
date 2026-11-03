package assessment

// An assessment is never changed: a reassessment adds a record. Readers that
// count quality take only the latest record of each assessed object.
//
// The object is the iteration (iteration_id) of an iteration assessment and
// the task (task_key) of a task assessment, whose iteration_id is empty. The
// latest record of an object has the greatest created_at; at an equal
// created_at, the latest rubric_version; at an equal rubric_version, the
// greatest seq.
//
// A rubric_version of the form name/N (N one or more trailing digits, name
// not empty) compares name as a string and then N as a number, so
// iteration-review/10 is later than iteration-review/2 and
// iteration-review/1 is later than iteration-review. Any other value is a
// name without a number, which sorts before every number.

// IsLatest is an SQL condition on the assessments row aliased a: true when
// no other record of the same object is later. It looks at every record of
// the object, so the conditions it is combined with filter after the choice.
// Metrics and assessment.ls without history use it; the measurement query
// internal/assessment/testdata/measure.sql embeds it.
//
// It is a correlated NOT EXISTS rather than a window over the table: each
// row is checked through idx_assessments_iteration or idx_assessments_task,
// so ls with a limit reads few rows, and the rubric expressions run only on
// an equal created_at.
var IsLatest = `CASE WHEN a.iteration_id <> ''
	THEN NOT EXISTS (SELECT 1 FROM assessments b WHERE b.iteration_id = a.iteration_id AND ` + later + `)
	ELSE NOT EXISTS (SELECT 1 FROM assessments b WHERE b.iteration_id = '' AND b.task_key = a.task_key AND ` + later + `) END`

// later is true when record b of an object is later than record a. The row
// values compare element by element, left to right.
var later = `(b.created_at > a.created_at OR (b.created_at = a.created_at
		AND (` + rubricName("b") + `, ` + rubricNumber("b") + `, b.seq) > (` + rubricName("a") + `, ` + rubricNumber("a") + `, a.seq)))`

// rubricHead is rubric_version of alias t without its trailing digits.
func rubricHead(t string) string { return `rtrim(` + t + `.rubric_version, '0123456789')` }

// rubricNumbered is true for a rubric_version of the form name/N.
func rubricNumbered(t string) string {
	return rubricHead(t) + ` LIKE '_%/' AND length(` + rubricHead(t) + `) < length(` + t + `.rubric_version)`
}

func rubricName(t string) string {
	return `(CASE WHEN ` + rubricNumbered(t) + ` THEN substr(` + rubricHead(t) + `, 1, length(` + rubricHead(t) +
		`) - 1) ELSE ` + t + `.rubric_version END)`
}

// rubricNumber is N, or -1 without a number.
func rubricNumber(t string) string {
	return `(CASE WHEN ` + rubricNumbered(t) + ` THEN CAST(substr(` + t + `.rubric_version, length(` + rubricHead(t) +
		`) + 1) AS INTEGER) ELSE -1 END)`
}
