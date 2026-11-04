package selfprofile

import (
	"testing"

	"github.com/enowx/enowx/proto"
)

func TestReadVmmapSummary(t *testing.T) {
	var m proto.ProfileMemory
	readVmmap(`Physical footprint:         1.2G
Physical footprint (peak):  2.5G
                                VIRTUAL RESIDENT    DIRTY  SWAPPED
REGION TYPE                        SIZE     SIZE     SIZE     SIZE
===========                     ======= ========    =====  =======
MALLOC guard page                   32K       0K       0K       0K
VM_ALLOCATE                       5509M     900M     880M      10M
__TEXT                             411M     120M       0K       0K
TOTAL                             6000M    1020M     880M      10M
`, &m)
	if m.Footprint == 0 || m.FootprintPeak == 0 || len(m.Regions) != 2 || m.Regions[0].Name != "VM_ALLOCATE" || m.Regions[0].RSS != 900<<20 {
		t.Fatalf("%+v", m)
	}
}
