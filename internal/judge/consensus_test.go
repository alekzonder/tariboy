package judge

import "testing"

func TestConsensusMarksTieDisputedAndUsesMedian(t *testing.T) {
	got := Consensus([]AnalysisResult{{Verdict: "pass", Score: .9, Confidence: .8}, {Verdict: "fail", Score: .2, Confidence: .6}})
	if got.Verdict != "disputed" || got.Score != .55 || got.Confidence != .7 {
		t.Fatalf("consensus=%+v", got)
	}
}
