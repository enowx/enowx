//go:build !linux && !darwin

package selfprofile

import (
	"os"

	"github.com/enowx/enowx/core/procstat"
	"github.com/enowx/enowx/proto"
)

func pages() proto.ProfileMemory {
	var m proto.ProfileMemory
	list, err := procstat.Snapshot()
	if err != nil {
		m.Note = err.Error()
		return m
	}
	for _, p := range list {
		if p.PID == os.Getpid() {
			m.RSS = p.RSS
		}
	}
	return m
}
