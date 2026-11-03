package proctree

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Read takes one snapshot of the process table from /proc. It fills Args from
// /proc/<pid>/cmdline only for the direct children of roots. A process that
// exits during the read is left out.
func Read(roots []int) ([]Proc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	procs := make([]Proc, 0, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		p, err := parseStat(pid, string(stat))
		if err != nil {
			return nil, err
		}
		procs = append(procs, p)
	}
	for i := range procs {
		if isChildOf(procs[i], roots) {
			procs[i].Args = readCmdline(procs[i].PID)
		}
	}
	return procs, nil
}

// parseStat reads ppid, state and comm from /proc/<pid>/stat. comm can hold
// spaces and parentheses, so it ends at the last ')'.
func parseStat(pid int, stat string) (Proc, error) {
	open, end := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return Proc{}, fmt.Errorf("proc %d: malformed stat", pid)
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 2 {
		return Proc{}, fmt.Errorf("proc %d: malformed stat", pid)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return Proc{}, fmt.Errorf("proc %d: ppid: %w", pid, err)
	}
	return Proc{PID: pid, PPID: ppid, Zombie: fields[0] == "Z", Comm: stat[open+1 : end]}, nil
}

func readCmdline(pid int) []string {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil || len(raw) == 0 {
		return nil
	}
	return strings.Split(string(bytes.TrimSuffix(raw, []byte{0})), "\x00")
}
