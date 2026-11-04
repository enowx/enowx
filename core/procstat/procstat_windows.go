package procstat

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no call that lists a process's environment or arguments for another
// process, but each lives in that process's memory, its user may read. The environment
// is how a process is told as a session's here: ENOWX_SESSION, inherited by all it
// starts, even those that leave its console.

const queryRights = windows.PROCESS_QUERY_LIMITED_INFORMATION

// What golang.org/x/sys/windows doesn't wrap, from kernel32.
var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procMemoryInfo   = kernel32.NewProc("K32GetProcessMemoryInfo")
	procMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryCounters is PROCESS_MEMORY_COUNTERS.
type memoryCounters struct {
	size, pageFaults                 uint32
	peakWorkingSet, workingSet       uintptr
	quotaPeakPaged, quotaPaged       uintptr
	quotaPeakNonPaged, quotaNonPaged uintptr
	pagefile, peakPagefile           uintptr
}

// memoryStatus is MEMORYSTATUSEX.
type memoryStatus struct {
	size, memoryLoad                                                       uint32
	totalPhys, availPhys, totalPage, availPage, totalVirtual, availVirtual uint64
	availExtended                                                          uint64
}

func processes() ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []Proc
	born := map[int]uint64{}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		p := Proc{PID: int(e.ProcessID), PPID: int(e.ParentProcessID), Name: windows.UTF16ToString(e.ExeFile[:])}
		if created, cpu, rss, ok := timesOf(p.PID); ok {
			p.CPUTime, p.RSS, p.Start = cpu, rss, strconv.FormatUint(created, 10)
			born[p.PID] = created
		}
		out = append(out, p)
	}
	dropStaleParents(out, born)
	return out, nil
}

// dropStaleParents clears a parent that can't be: Windows doesn't move a process to a
// new parent when its own ends, so its parent's PID may by now be another's, started
// after it. Taken for its child, that would be stopped along with what isn't its own.
func dropStaleParents(list []Proc, born map[int]uint64) {
	for i := range list {
		parent, known := born[list[i].PPID]
		if !known || parent > born[list[i].PID] {
			list[i].PPID = 0
		}
	}
}

// timesOf is when pid started (a FILETIME), the CPU it has used and its memory.
func timesOf(pid int) (created uint64, cpu time.Duration, rss uint64, ok bool) {
	h, err := windows.OpenProcess(queryRights, false, uint32(pid))
	if err != nil {
		return 0, 0, 0, false
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &e, &k, &u) != nil {
		return 0, 0, 0, false
	}
	var m memoryCounters
	m.size = uint32(unsafe.Sizeof(m))
	if r, _, _ := procMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.size)); r != 0 {
		rss = uint64(m.workingSet)
	}
	// FILETIMEs count hundreds of nanoseconds.
	cpu = time.Duration(fileTicks(k)+fileTicks(u)) * 100
	return fileTicks(c), cpu, rss, true
}

func fileTicks(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }

func memory() (total, used uint64) {
	var m memoryStatus
	m.size = uint32(unsafe.Sizeof(m))
	if r, _, _ := procMemoryStatus.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return m.totalPhys, m.totalPhys - m.availPhys
}

// readParams reads pid's process parameters, and gives what f wants read of them.
func readParams(pid int, f func(h windows.Handle, p windows.RTL_USER_PROCESS_PARAMETERS)) {
	h, err := windows.OpenProcess(queryRights|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	var info windows.PROCESS_BASIC_INFORMATION
	var n uint32
	if windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &n) != nil || info.PebBaseAddress == nil {
		return
	}
	var peb windows.PEB
	if !readInto(h, uintptr(unsafe.Pointer(info.PebBaseAddress)), unsafe.Pointer(&peb), unsafe.Sizeof(peb)) || peb.ProcessParameters == nil {
		return
	}
	var params windows.RTL_USER_PROCESS_PARAMETERS
	if readInto(h, uintptr(unsafe.Pointer(peb.ProcessParameters)), unsafe.Pointer(&params), unsafe.Sizeof(params)) {
		f(h, params)
	}
}

func readInto(h windows.Handle, from uintptr, to unsafe.Pointer, size uintptr) bool {
	var got uintptr
	return windows.ReadProcessMemory(h, from, (*byte)(to), size, &got) == nil && got == size
}

// readUTF16 reads n bytes of UTF-16 from another process.
func readUTF16(h windows.Handle, from uintptr, n uintptr) []uint16 {
	const most = 1 << 20 // an environment is far smaller; a bad size isn't read
	if from == 0 || n < 2 || n > most {
		return nil
	}
	buf := make([]uint16, n/2)
	var got uintptr
	if windows.ReadProcessMemory(h, from, (*byte)(unsafe.Pointer(&buf[0])), n, &got) != nil {
		return nil
	}
	return buf[:got/2]
}

// Env returns the value of key in pid's environment, where this user may read it.
func Env(pid int, key string) string {
	value := ""
	readParams(pid, func(h windows.Handle, p windows.RTL_USER_PROCESS_PARAMETERS) {
		value = envValue(readUTF16(h, uintptr(p.Environment), p.EnvironmentSize), key)
	})
	return value
}

// envValue finds key in an environment block: NAME=value, each ended by a NUL, names
// in any case.
func envValue(block []uint16, key string) string {
	for len(block) > 0 {
		end := slices.Index(block, 0)
		if end <= 0 {
			break
		}
		entry := string(utf16.Decode(block[:end]))
		block = block[end+1:]
		if name, value, ok := strings.Cut(entry, "="); ok && strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}

// Args returns pid's arguments, its program first.
func Args(pid int) []string {
	var args []string
	readParams(pid, func(h windows.Handle, p windows.RTL_USER_PROCESS_PARAMETERS) {
		line := readUTF16(h, uintptr(unsafe.Pointer(p.CommandLine.Buffer)), uintptr(p.CommandLine.Length))
		if len(line) == 0 {
			return
		}
		args, _ = windows.DecomposeCommandLine(string(utf16.Decode(line)))
	})
	return args
}

func process(pid int) (Proc, error) {
	list, err := processes()
	if err != nil {
		return Proc{}, err
	}
	for _, p := range list {
		if p.PID == pid {
			return p, nil
		}
	}
	return Proc{}, errors.New("procstat: no such process")
}
