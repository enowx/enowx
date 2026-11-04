package procstat

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func processes() ([]Proc, error) {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,rss=,time=,lstart=,comm=").Output()
	if err != nil {
		return nil, err
	}
	return parsePS(string(out)), nil
}

func process(pid int) (Proc, error) {
	out, err := exec.Command("ps", "-o", "pid=,ppid=,rss=,time=,lstart=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return Proc{}, err
	}
	list := parsePS(string(out))
	if len(list) == 0 {
		return Proc{}, errors.New("procstat: no such process")
	}
	return list[0], nil
}

// parsePS reads ps's lines: pid, ppid, rss, time, a start of five words (Tue Nov  3
// 04:08:00 2026), and a program's path, which may hold spaces.
func parsePS(out string) []Proc {
	var list []Proc
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		rss, _ := strconv.ParseUint(f[2], 10, 64)
		list = append(list, Proc{PID: pid, PPID: ppid, RSS: rss << 10, CPUTime: parseCPUTime(f[3]),
			Start: strings.Join(f[4:9], " "), Name: filepath.Base(strings.Join(f[9:], " "))})
	}
	return list
}

// parseCPUTime reads ps's time: [[dd-]hh:]mm:ss.cc.
func parseCPUTime(s string) time.Duration {
	var days int
	if d, rest, ok := strings.Cut(s, "-"); ok {
		days, _ = strconv.Atoi(d)
		s = rest
	}
	parts := strings.Split(s, ":")
	var total float64
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		total = total*60 + v
	}
	return time.Duration((total + float64(days)*86400) * float64(time.Second))
}

func memory() (total, used uint64) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0
	}
	total, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	vm, err := exec.Command("vm_stat").Output()
	if err != nil {
		return total, 0
	}
	page := uint64(4096)
	var free uint64
	for _, line := range strings.Split(string(vm), "\n") {
		if i := strings.Index(line, "page size of "); i >= 0 {
			if n, err := strconv.ParseUint(strings.Fields(line[i+13:])[0], 10, 64); err == nil {
				page = n
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), "."), 10, 64)
		switch k {
		case "Pages free", "Pages inactive", "Pages speculative", "Pages purgeable":
			free += n
		}
	}
	if free*page > total {
		return total, 0
	}
	return total, total - free*page
}

// procArgs reads pid's arguments and environment as the kernel gives
// them to the process's own user (kern.procargs2: argc, the executable's
// path, padding, then each argument and each variable NUL-terminated).
// Recent macOS leaves the environment out.
func procArgs(pid int) (args, env []string) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(buf) < 4 {
		return nil, nil
	}
	argc := int(binary.LittleEndian.Uint32(buf))
	rest := buf[4:]
	if i := bytes.IndexByte(rest, 0); i >= 0 {
		rest = rest[i:] // past the executable's path
	}
	fields := strings.Split(strings.TrimLeft(string(rest), "\x00"), "\x00")
	if len(fields) < argc {
		return fields, nil
	}
	for _, f := range fields[argc:] {
		if f == "" {
			break // the environment ends
		}
		env = append(env, f)
	}
	return fields[:argc], env
}

// Args returns pid's arguments, its program first.
func Args(pid int) []string {
	args, _ := procArgs(pid)
	return args
}

// Env returns the value of key in pid's environment, where the system
// shows it.
func Env(pid int, key string) string {
	_, env := procArgs(pid)
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}
