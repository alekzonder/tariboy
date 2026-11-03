package proctree

import "testing"

func TestReadFindsLiveShellCommandLinux(t *testing.T) {
	testReadFindsShellCommand(t, "sleep 30")
}

func TestParseStatCommWithParentheses(t *testing.T) {
	p, err := parseStat(42, "42 (a) (b) Z 7 42 42 0 -1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Comm != "a) (b" || p.PPID != 7 || !p.Zombie {
		t.Fatalf("parsed %+v", p)
	}
}
