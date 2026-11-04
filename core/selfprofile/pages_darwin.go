package selfprofile

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

// pages asks vmmap, the system's tool, as macOS's physical footprint has no call a
// program without cgo can make. It stops this process for as long as it reads, at most
// about a second, so it runs only when a profile is asked for.
func pages() proto.ProfileMemory {
	var m proto.ProfileMemory
	if list, err := procstat.Snapshot(); err == nil {
		for _, p := range list {
			if p.PID == os.Getpid() {
				m.RSS = p.RSS
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := quietcmd.CommandContext(ctx, "vmmap", "-summary", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		m.Note = "vmmap: " + err.Error()
		return m
	}
	readVmmap(string(out), &m)
	return m
}

// readVmmap reads vmmap -summary: its footprint lines, and the table of regions
// whose columns are virtual, resident, dirty, swapped.
func readVmmap(text string, m *proto.ProfileMemory) {
	inTable := false
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "Physical footprint:"); ok {
			m.Footprint = vmmapSize(rest)
			continue
		}
		if rest, ok := strings.CutPrefix(line, "Physical footprint (peak):"); ok {
			m.FootprintPeak = vmmapSize(rest)
			continue
		}
		if strings.HasPrefix(line, "REGION TYPE") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if strings.HasPrefix(line, "TOTAL") {
			break
		}
		fields := strings.Fields(line)
		// A region's name may hold spaces; its sizes come right after it.
		for i, f := range fields {
			if vmmapSize(f) > 0 || f == "0K" {
				if i > 0 && i+2 < len(fields) {
					r := proto.ProfileRegion{Name: strings.Join(fields[:i], " "), RSS: vmmapSize(fields[i+1]), PrivateDirty: vmmapSize(fields[i+2])}
					if r.RSS >= 1<<20 {
						m.Regions = append(m.Regions, r)
					}
				}
				break
			}
		}
	}
}

// vmmapSize reads 12.5M, 1.2G or 512K.
func vmmapSize(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	unit := uint64(1)
	switch s[len(s)-1] {
	case 'K':
		unit = 1 << 10
	case 'M':
		unit = 1 << 20
	case 'G':
		unit = 1 << 30
	default:
		return 0
	}
	n, err := strconv.ParseFloat(s[:len(s)-1], 64)
	if err != nil {
		return 0
	}
	return uint64(n * float64(unit))
}
