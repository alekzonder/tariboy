package judge

import "sort"

func Consensus(results []AnalysisResult) TargetConsensus {
	if len(results) == 0 {
		return TargetConsensus{}
	}
	pass, fail, uncertain := 0, 0, 0
	scores, confidences := make([]float64, 0, len(results)), make([]float64, 0, len(results))
	min, max := results[0].Score, results[0].Score
	for _, r := range results {
		scores, confidences = append(scores, r.Score), append(confidences, r.Confidence)
		if r.Score < min {
			min = r.Score
		}
		if r.Score > max {
			max = r.Score
		}
		switch r.Verdict {
		case "pass":
			pass++
		case "fail":
			fail++
		default:
			uncertain++
		}
	}
	v := "disputed"
	if pass > fail && pass > uncertain {
		v = "pass"
	}
	if fail > pass && fail > uncertain {
		v = "fail"
	}
	if max-min >= .5 {
		v = "disputed"
	}
	return TargetConsensus{Verdict: v, Score: median(scores), Confidence: median(confidences)}
}
func median(v []float64) float64 {
	sort.Float64s(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}
