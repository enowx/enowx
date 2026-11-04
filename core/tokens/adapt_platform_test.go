package tokens

import (
	"path/filepath"
	"testing"
)

func TestPlatformAdapterCountsAnswers(t *testing.T) {
	t.Setenv("ENOWX_HOME", "")
	home := t.TempDir()
	data := filepath.Join(home, ".enowx-app")
	const conv = `{"type":"meta","conversation":"c1","dir":"/work/app","model":"claude-opus-5-5"}
{"type":"user","text":"hi"}
{"type":"delta","msg":"m1","text":"hel"}
{"type":"assistant","time":"2026-10-30T10:00:00Z","msg":"m1","usage":{"input":10,"output":20,"cacheRead":30,"cacheWrite":40},"cost":0.5,"totalCost":0.5}
{"type":"assistant","time":"2026-10-30T10:01:00Z","msg":"m2","sidechain":"t1","model":"claude-haiku-5-5","usage":{"input":1,"output":2,"cacheRead":0,"cacheWrite":0},"cost":0.01,"totalCost":0.51}
{"type":"assistant","time":"2026-10-30T10:02:00Z","msg":"m3"}
`
	session := filepath.Join(data, "agent-sessions", "s1", "c1.jsonl")
	byHand := filepath.Join(data, "claude-platform", "sessions", "c2.jsonl")
	writeFile(t, session, conv)
	writeFile(t, byHand, `{"type":"assistant","time":"2026-10-30T11:00:00Z","msg":"m9","model":"claude-sonnet-5-5","usage":{"input":5,"output":5,"cacheRead":0,"cacheWrite":0}}`+"\n")
	// An agent sharing the stores' folder (OMP) counts for nothing here.
	writeFile(t, filepath.Join(data, "agent-sessions", "s2", "omp.jsonl"), `{"type":"message","message":{"role":"assistant","usage":{"input":7}}}`+"\n")

	if got := platformFiles(Env{Home: home}); len(got) != 3 {
		t.Fatalf("files: %v", got)
	}
	st := &FileState{}
	calls, err := readPlatform(session, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls: %+v", calls)
	}
	c := calls[0]
	if c.ID != "cp:c1:m1" || c.Model != "claude-opus-5-5" || c.Provider != "anthropic" || c.Dir != "/work/app" || c.Conversation != "c1" ||
		c.Input != 10 || c.Output != 20 || c.CacheRead != 30 || c.CacheWrite != 40 || c.Cost != 0.5 || c.ModelInferred {
		t.Fatalf("first: %+v", c)
	}
	if calls[1].Model != "claude-haiku-5-5" {
		t.Fatalf("a subagent's: %+v", calls[1])
	}
	// Read on from where it was: nothing new, nothing twice.
	again, err := readPlatform(session, st)
	if err != nil || len(again) != 0 {
		t.Fatalf("again: %+v, %v", again, err)
	}
	other, err := readPlatform(byHand, &FileState{})
	if err != nil || len(other) != 1 || other[0].ID != "cp:c2:m9" {
		t.Fatalf("by hand: %+v, %v", other, err)
	}
	if LayerOf("claude-platform") != "records" {
		t.Fatalf("layer: %s", LayerOf("claude-platform"))
	}
}

func TestPlatformAdapterCountsCompactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c1.jsonl")
	writeFile(t, path, `{"type":"meta","conversation":"c1","dir":"/w","model":"claude-opus-5-5"}
{"type":"compact","time":"2026-10-30T10:00:00Z","msg":"m1","usage":{"input":10,"output":20,"cacheRead":0,"cacheWrite":0},"cost":0.5,"totalCost":0.5}
{"type":"compact","time":"2026-10-30T10:01:00Z","usage":{"input":1,"output":2,"cacheRead":0,"cacheWrite":0},"cost":0.1,"totalCost":0.6}
{"type":"compact","time":"2026-10-30T10:02:00Z"}
`)
	calls, err := readPlatform(path, &FileState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Input != 10 || calls[0].Cost != 0.5 || calls[1].Output != 2 || calls[0].ID == calls[1].ID || calls[0].Model != "claude-opus-5-5" {
		t.Fatalf("calls: %+v", calls)
	}
}
