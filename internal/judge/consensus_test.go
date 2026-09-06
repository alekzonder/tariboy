package judge

import "testing"

func TestConsensusMarksTieDisputedAndUsesMedian(t *testing.T) {
	got := Consensus([]AnalysisResult{{Verdict: "pass", Score: .9, Confidence: .8}, {Verdict: "fail", Score: .2, Confidence: .6}})
	if got.Verdict != "disputed" || got.Score != .55 || got.Confidence != .7 {
		t.Fatalf("consensus=%+v", got)
	}
}

func TestConsensusKeepsAllUncertainEvenWhenScoresSpread(t *testing.T) {
	got := Consensus([]AnalysisResult{{Verdict: "uncertain", Score: .1, Confidence: .3}, {Verdict: "uncertain", Score: .9, Confidence: .7}})
	if got.Verdict != "uncertain" || got.Score != .5 || got.Confidence != .5 {
		t.Fatalf("consensus=%+v", got)
	}
}
