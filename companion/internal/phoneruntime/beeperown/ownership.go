package beeperown

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

type Expected struct {
	ChildPID  int
	StartTime int64
	ExePath   string
	ExeSHA256 string
	Port      int
}

type ProcSnapshot struct {
	Stat   string
	Exe    string
	SHA256 string
	FDs    map[string]string
}

type MemoryFS struct {
	ProcNetTCP string
	Procs      map[string]ProcSnapshot
}

func Prove(fs MemoryFS, want Expected) error {
	pidKey := strconv.Itoa(want.ChildPID)
	proc, ok := fs.Procs[pidKey]
	if !ok {
		return fmt.Errorf("beeper ownership: child pid %d missing", want.ChildPID)
	}
	fields := strings.Fields(proc.Stat)
	if len(fields) < 22 {
		return fmt.Errorf("beeper ownership: malformed /proc/%d/stat", want.ChildPID)
	}
	start, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return err
	}
	if start != want.StartTime {
		return fmt.Errorf("beeper ownership: start time mismatch")
	}
	if proc.Exe != want.ExePath {
		return fmt.Errorf("beeper ownership: executable path mismatch")
	}
	if proc.SHA256 != want.ExeSHA256 {
		return fmt.Errorf("beeper ownership: executable hash mismatch")
	}

	portHex := strings.ToUpper(fmt.Sprintf("%04X", want.Port))
	ownedSockets := map[string]bool{}
	for _, target := range proc.FDs {
		if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
			inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			ownedSockets[inode] = true
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(fs.ProcNetTCP))
	matches := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "sl ") || line == "sl" {
			continue
		}
		cols := strings.Fields(line)
		if len(cols) < 10 {
			continue
		}
		local := cols[1]
		state := cols[3]
		inode := cols[9]
		if state != "0A" {
			continue
		}
		parts := strings.Split(local, ":")
		if len(parts) != 2 {
			continue
		}
		if !strings.EqualFold(parts[0], "0100007F") {
			continue
		}
		if !strings.EqualFold(parts[1], portHex) {
			continue
		}
		if !ownedSockets[inode] {
			return fmt.Errorf("beeper ownership: listener on %d owned by foreign inode %s", want.Port, inode)
		}
		matches++
	}
	if matches != 1 {
		return fmt.Errorf("beeper ownership: want exactly one owned listener on %d, got %d", want.Port, matches)
	}
	return nil
}
