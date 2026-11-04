package selfprofile

import (
	"context"
	"os"
	"strconv"
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
