package proctree

import (
	"bytes"
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// szomb is SZOMB from <sys/proc.h>.
const szomb = 5

// Read takes one snapshot of the process table with the kern.proc.all sysctl.
// It fills Args from kern.procargs2 only for the direct children of roots.
func Read(roots []int) ([]Proc, error) {
	kinfos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	procs := make([]Proc, 0, len(kinfos))
	for _, k := range kinfos {
		p := Proc{
			PID:    int(k.Proc.P_pid),
			PPID:   int(k.Eproc.Ppid),
			Zombie: k.Proc.P_stat == szomb,
			Comm:   unix.ByteSliceToString(k.Proc.P_comm[:]),
		}
		if isChildOf(p, roots) {
			p.Args = readProcArgs(p.PID)
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// readProcArgs parses kern.procargs2: a 32-bit argc, the executable path, NUL
// padding, then argc NUL-terminated arguments.
func readProcArgs(pid int) []string {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return nil
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	rest := raw[4:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return nil
	}
	rest = bytes.TrimLeft(rest[end:], "\x00")
	args := make([]string, 0, argc)
	for len(args) < argc && len(rest) > 0 {
		end := bytes.IndexByte(rest, 0)
		if end < 0 {
			end = len(rest)
		}
		args = append(args, string(rest[:end]))
		rest = rest[min(end+1, len(rest)):]
	}
	return args
}
