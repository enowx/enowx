package selfprofile

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

func pages() proto.ProfileMemory {
	var m proto.ProfileMemory
	f, err := os.Open("/proc/self/smaps")
	if err != nil {
		m.Note = "smaps: " + err.Error()
		return m
	}
	defer f.Close()
	m = readSmaps(f)
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "VmHWM:"); ok {
				m.PeakRSS = kb(rest)
			}
		}
	}
	return m
}

// readSmaps sums a process's mappings, by file, the rest as anonymous memory.
func readSmaps(f *os.File) proto.ProfileMemory {
	var m proto.ProfileMemory
	regions := map[string]*proto.ProfileRegion{}
	var at *proto.ProfileRegion
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 1<<20)
	for scan.Scan() {
		line := scan.Text()
		key, rest, ok := strings.Cut(line, ":")
		if !ok || strings.ContainsAny(key, " -") { // a mapping's first line
			if fields := strings.Fields(line); len(fields) >= 5 {
				name := "anonymous"
				if len(fields) >= 6 {
					name = strings.Join(fields[5:], " ") // the runtime's "[anon: Go: heap]"
					if strings.HasPrefix(name, "/") {
						name = filepath.Base(name)
					}
				}
				at = regions[name]
				if at == nil {
					at = &proto.ProfileRegion{Name: name}
					regions[name] = at
				}
			}
			continue
		}
		switch key {
		case "Rss":
			n := kb(rest)
			m.RSS += n
			if at != nil {
				at.RSS += n
			}
		case "Pss":
			m.PSS += kb(rest)
		case "Private_Dirty":
			n := kb(rest)
			m.PrivateDirty += n
			if at != nil {
				at.PrivateDirty += n
			}
		case "Private_Clean":
			m.PrivateClean += kb(rest)
		case "Shared_Clean":
			m.SharedClean += kb(rest)
		case "Shared_Dirty":
			m.SharedDirty += kb(rest)
		case "Anonymous":
			m.Anon += kb(rest)
		case "Swap":
			m.Swap += kb(rest)
		}
	}
	for _, r := range regions {
		if r.RSS >= 1<<20 {
			m.Regions = append(m.Regions, *r)
		}
	}
	slices.SortFunc(m.Regions, func(a, b proto.ProfileRegion) int {
		if a.RSS != b.RSS {
			if a.RSS > b.RSS {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	m.Regions = m.Regions[:min(len(m.Regions), 15)]
	return m
}

// kb reads smaps' " 1234 kB" as bytes.
func kb(s string) uint64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[0], 10, 64)
	return n << 10
}
