package agent

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/procstat"
)

func TestGOACodexConsoleAncestry(t *testing.T) {
	cli := `C:\tools\codex\bin`
	runtime := `C:\fixture\AppData\Local\OpenAI\Codex\runtimes\cua_node\abc\bin`
	images := map[int]string{
		1: cli + `\codex.exe`, 2: cli + `\codex-code-mode-host.exe`,
		3: `C:\Windows\System32\conhost.exe`, 4: runtime + `\node_repl.exe`,
		5: runtime + `\node.exe`, 6: `C:\Windows\System32\conhost.exe`,
	}
	argv := map[int][]string{}
	for pid, path := range images {
		argv[pid] = []string{path}
	}
	argv[3] = []string{`\??\C:\Windows\System32\conhost.exe`, "0x4"}
	argv[6] = argv[3]
	argv[5] = []string{images[5], "--import", "file:///C:/fixture/.codex/plugins/cache/openai-bundled/unified-computer-use/26/require-browser-access.mjs", filepath.Join(runtime, "node_modules", "@oai", "cua-repl", "bin", "cua-repl.mjs")}
	list := []procstat.Proc{
		{PID: 1, Start: "100"}, {PID: 2, PPID: 1, Start: "101"},
		{PID: 3, PPID: 2, Start: "102"}, {PID: 4, PPID: 1, Start: "103"},
		{PID: 5, PPID: 1, Start: "104"}, {PID: 6, PPID: 5, Start: "105"},
	}
	for i := range list {
		list[i].Name = filepath.Base(images[list[i].PID])
	}
	classify := func(list []procstat.Proc, tagged func(int) bool) map[int]bool {
		return WindowsGOAProcesses("codex", "goa", list, func(pid int) []string { return argv[pid] },
			func(pid int) string { return images[pid] }, tagged, `C:\fixture\AppData\Local`, `C:\Windows`)
	}
	allTagged := func(int) bool { return true }
	for _, pid := range []int{2, 3, 4, 5, 6} {
		if !classify(list, allTagged)[pid] {
			t.Fatalf("known helper %d was flagged", pid)
		}
	}
	for _, tc := range []struct {
		name   string
		change func([]procstat.Proc)
	}{
		{"unrelated console", func(ps []procstat.Proc) { ps[2].PPID = 1 }},
		{"missing parent", func(ps []procstat.Proc) { ps[2].PPID = 999 }},
		{"reused parent PID", func(ps []procstat.Proc) { ps[1].Start = "999" }},
		{"unknown start", func(ps []procstat.Proc) { ps[1].Start = "" }},
		{"missing console start", func(ps []procstat.Proc) { ps[2].Start = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := append([]procstat.Proc(nil), list...)
			tc.change(ps)
			if classify(ps, allTagged)[3] {
				t.Fatal("unproven console ancestry was exempted")
			}
		})
	}
	for _, tc := range []struct {
		name string
		pid  int
		path string
		args []string
	}{
		{"copied console", 3, `C:\temp\codex\conhost.exe`, argv[3]},
		{"console with other command", 3, images[3], []string{images[3], "arbitrary"}},
		{"copied helper", 2, `C:\temp\codex-code-mode-host.exe`, []string{`C:\temp\codex-code-mode-host.exe`}},
		{"helper with command", 2, images[2], []string{images[2], "run", "powershell"}},
		{"arbitrary node script", 5, images[5], []string{images[5], `C:\work\task.js`}},
		{"arbitrary node import", 5, images[5], []string{images[5], "--import", "file:///C:/work/task.mjs", argv[5][3]}},
		{"lookalike runtime", 5, strings.Replace(images[5], "cua_node", "cua_node_evil", 1), argv[5]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, a := images[tc.pid], argv[tc.pid]
			defer func() { images[tc.pid], argv[tc.pid] = path, a }()
			images[tc.pid], argv[tc.pid] = tc.path, tc.args
			got := classify(list, allTagged)
			if got[tc.pid] {
				t.Fatal("unknown process exempted")
			}
			if tc.pid == 2 && got[3] || tc.pid == 5 && got[6] {
				t.Fatal("unknown parent's console exempted")
			}
		})
	}
	if classify(list, func(pid int) bool { return pid != 1 })[3] {
		t.Fatal("another session's Codex accepted")
	}
	images[7], argv[7] = images[3], argv[3]
	ps := append(append([]procstat.Proc(nil), list...), procstat.Proc{PID: 7, PPID: 3, Name: "conhost.exe", Start: "106"})
	if classify(ps, allTagged)[7] {
		t.Fatal("console exemption propagated through descendants")
	}

}

func TestGOACodexNpmLauncherAncestry(t *testing.T) {
	pkg := `C:\npm\node_modules\@openai\codex`
	images := map[int]string{1: `C:\npm\node.exe`, 2: pkg + `\vendor\bin\codex.exe`, 3: `C:\Windows\System32\conhost.exe`}
	argv := map[int][]string{1: {images[1], pkg + `\bin\codex.js`, "--sandbox", "workspace-write"}, 2: {images[2]}, 3: {images[3], "0x4"}}
	ps := []procstat.Proc{{PID: 1, Name: "node.exe", Start: "10"}, {PID: 2, PPID: 1, Name: "codex.exe", Start: "11"}, {PID: 3, PPID: 1, Name: "conhost.exe", Start: "12"}}
	run := func() map[int]bool {
		return WindowsGOAProcesses("codex", "goa", ps, func(pid int) []string { return argv[pid] }, func(pid int) string { return images[pid] }, func(int) bool { return true }, `C:\fixture\AppData\Local`, `C:\Windows`)
	}
	if got := run(); !got[1] || !got[3] {
		t.Fatal("npm launcher or its console was flagged")
	}
	argv[1][1] = `C:\work\codex.js`
	if got := run(); got[1] || got[3] {
		t.Fatal("an unrelated Node script or its console was exempted")
	}
	argv[1][1] = pkg + `\bin\codex.js`
	images[2] = `C:\other\codex.exe`
	if run()[1] {
		t.Fatal("an unrelated native binary vouched for the launcher")
	}
}
