package procstat

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicks is USER_HZ, which Linux fixes at 100 for /proc.
const clockTicks = 100

func processes() ([]Proc, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []Proc
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if p, err := process(pid); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func process(pid int) (Proc, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return Proc{}, err
	}
	// The command name may hold spaces and parentheses: fields start
	// after the last ')'.
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return Proc{}, errors.New("procstat: unreadable stat")
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 22 {
		return Proc{}, errors.New("procstat: unreadable stat")
	}
	ppid, _ := strconv.Atoi(f[1])
	utime, _ := strconv.ParseUint(f[11], 10, 64)
	stime, _ := strconv.ParseUint(f[12], 10, 64)
	rss, _ := strconv.ParseUint(f[21], 10, 64)
	name := ""
	if open := strings.IndexByte(s, '('); open >= 0 && open < i {
		name = s[open+1 : i]
	}
	return Proc{
		PID: pid, PPID: ppid, RSS: rss * uint64(os.Getpagesize()),
		CPUTime: time.Duration(utime+stime) * time.Second / clockTicks,
		Name:    name, Start: f[19],
	}, nil
}

func memory() (total, used uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var avail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		switch k {
		case "MemTotal":
			total = n << 10
		case "MemAvailable":
			avail = n << 10
		}
	}
	if avail > total {
		avail = total
	}
	return total, total - avail
}

// Env returns the value of key in the environment pid started with.
func Env(pid int, key string) string {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return ""
	}
	for _, kv := range strings.Split(string(data), "\x00") {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

// Args returns pid's arguments, its program first.
func Args(pid int) []string {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
}
