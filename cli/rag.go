package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/enowx/enowx/proto"
)

const ragUsage = `usage: enowx-app rag [search QUERY [--k N] [--scope code,docs,notes,memory] | remember TEXT | memories | forget ID | status | index | plan | settings | set FIELD=VALUE… | project on agreed|off] [--project DIR]`

// ragCmd runs `enowx rag …`: the project found by meaning. In a session,
// its own project; elsewhere, the current folder's (or --project's).
func (r runner) ragCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(ragUsage)
	}
	fs := flag.NewFlagSet("rag", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	k := fs.Int("k", 5, "")
	scope := fs.String("scope", "", "")
	project := fs.String("project", "", "")
	sub := args[0]
	var words []string
	rest := args[1:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return errors.New(ragUsage)
		}
		rest = fs.Args()
		if len(rest) > 0 {
			words, rest = append(words, rest[0]), rest[1:]
		}
	}
	ref := proto.RAGRef{Session: os.Getenv(EnvSession)}
	if *project != "" || ref.Session == "" {
		dir := *project
		if dir == "" {
			dir, _ = os.Getwd()
		}
		abs, _ := filepath.Abs(dir)
		ref.Project = abs
	}
	text := strings.Join(words, " ")
	switch sub {
	case "search", "find":
		if text == "" {
			return errors.New(ragUsage)
		}
		var hits []proto.RAGHit
		p := proto.RAGSearchParams{RAGRef: ref, Query: text, K: *k}
		if *scope != "" {
			p.Scope = strings.Split(*scope, ",")
		}
		if err := r.c.RAG(proto.MethodRAGSearch, p, &hits); err != nil {
			return err
		}
		if len(hits) == 0 {
			fmt.Fprintln(r.out, "Nothing found.")
		}
		for _, h := range hits {
			where := h.File
			if h.Start > 0 {
				where += fmt.Sprintf(":%d-%d", h.Start, h.End)
			}
			if h.Symbol != "" {
				where += " (" + h.Symbol + ")"
			}
			if where == "" {
				where = h.Source
			}
			if h.Memory != "" {
				where = "memory " + h.Memory
			}
			fmt.Fprintf(r.out, "%s  [%s %.3f]\n%s\n\n", where, h.Source, h.Score, indent(h.Snippet))
		}
		return nil
	case "remember":
		if text == "" {
			return errors.New(ragUsage)
		}
		var m proto.RAGMemory
		if err := r.c.RAG(proto.MethodRAGRemember, proto.RAGRememberParams{RAGRef: ref, Text: text}, &m); err != nil {
			return err
		}
		fmt.Fprintln(r.out, "Kept as memory", m.ID)
		return nil
	case "memories":
		var list []proto.RAGMemory
		if err := r.c.RAG(proto.MethodRAGMemories, ref, &list); err != nil {
			return err
		}
		for _, m := range list {
			fmt.Fprintf(r.out, "%s  %s  %s\n", m.ID, m.Time.Local().Format("Jan 2 15:04"), m.Text)
		}
		return nil
	case "forget":
		if len(words) != 1 {
			return errors.New(ragUsage)
		}
		return r.c.RAG(proto.MethodRAGForget, proto.RAGForgetParams{RAGRef: ref, ID: words[0]}, nil)
	case "settings":
		return r.ragSettingsShow()
	case "set":
		return r.ragSettingsSet(words)
	case "plan":
		var plan proto.RAGPlan
		if err := r.c.RAG(proto.MethodRAGPlan, ref, &plan); err != nil {
			return err
		}
		return writeJSON(r.out, plan)
	case "project":
		return r.ragProject(ref, words)
	case "status", "index":
		var st proto.RAGStatus
		method := proto.MethodRAGStatus
		if sub == "index" {
			method = proto.MethodRAGIndex
		}
		if err := r.c.RAG(method, ref, &st); err != nil {
			return err
		}
		state := "off"
		switch {
		case !st.Ready:
			state = "no embedder set (Settings → RAG)"
		case st.Indexing:
			state = "indexing"
		case st.Settings.On:
			state = "on"
		}
		fmt.Fprintf(r.out, "%s: %s, %d chunks", st.Project, state, st.Chunks)
		if !st.Indexed.IsZero() {
			fmt.Fprintf(r.out, ", indexed %s", st.Indexed.Local().Format("Jan 2 15:04"))
		}
		if st.Error != "" {
			fmt.Fprintf(r.out, ", %s", st.Error)
		}
		fmt.Fprintln(r.out)
		return nil
	}
	return errors.New(ragUsage)
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n    ")
}

// ragTools are RAG as MCP tools: an agent's own project, found by meaning.
var ragTools = []mcpTool{
	{name: "rag_search", description: "Search this session's project by meaning (code, docs, attached notes, memories): file:line, symbol and the part found. Before reading widely, or changing something unfamiliar.",
		props: map[string]mcpProp{"query": {"string", "What you look for, in words"}, "k": {"number", "How many results (5)"},
			"scope": {"string", "Only these sources, comma-separated: code, docs, notes, memory"}},
		required: []string{"query"},
		args: func(a map[string]any) []string {
			return withFlag(withFlag([]string{"search", str(a, "query")}, a, "k", "k"), a, "scope", "scope")
		}},
	{name: "rag_remember", description: "Keep a decision or gotcha of this project as a memory, found by later searches and by other agents.",
		props: map[string]mcpProp{"text": {"string", "What to remember, in a sentence or two"}}, required: []string{"text"},
		args: func(a map[string]any) []string { return []string{"remember", str(a, "text")} }},
	{name: "rag_status", description: "Whether this session's project is indexed, how much, and when last.",
		args: func(map[string]any) []string { return []string{"status"} }},
}

// ragSettingsShow prints the host's RAG settings: no key's value is in them.
func (r runner) ragSettingsShow() error {
	var st proto.RAGSettings
	if err := r.c.RAG(proto.MethodRAGSettings, nil, &st); err != nil {
		return err
	}
	return writeJSON(r.out, st)
}

// ragSettingsSet changes settings by field: never a key, only a credential's name.
func (r runner) ragSettingsSet(fields []string) error {
	var st proto.RAGSettings
	if err := r.c.RAG(proto.MethodRAGSettings, nil, &st); err != nil {
		return err
	}
	if len(fields) == 0 {
		return errors.New(ragUsage)
	}
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return errors.New(ragUsage)
		}
		switch k {
		case "embedder":
			st.Embedder = v
		case "model":
			st.Model = v
		case "dim":
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("rag: dim: %w", err)
			}
			st.Dim = n
		case "base_url":
			st.BaseURL = v
		case "key_cred":
			st.KeyCred = v
		case "store":
			st.Store = v
		case "postgres_cred":
			st.PostgresCred = v
		default:
			return fmt.Errorf("rag: no setting %q: embedder, model, dim, base_url, key_cred, store, postgres_cred. A key's value is typed in the app", k)
		}
	}
	if err := r.c.RAG(proto.MethodRAGSetSettings, st, &st); err != nil {
		return err
	}
	return writeJSON(r.out, st)
}

// ragProject turns a project's index on or off. On sends its files to the
// embedder: the agent asks the user first, as its guide says.
func (r runner) ragProject(ref proto.RAGRef, words []string) error {
	if len(words) == 0 || words[0] != "on" && words[0] != "off" {
		return errors.New(ragUsage)
	}
	if words[0] == "on" && (len(words) != 2 || words[1] != "agreed") {
		return errors.New("rag: on sends the project's files to the embedder: ask the user, then: rag project on agreed --project DIR")
	}
	var st proto.RAGStatus
	if err := r.c.RAG(proto.MethodRAGStatus, ref, &st); err != nil {
		return err
	}
	set := st.Settings
	set.On = words[0] == "on"
	if set.On {
		set.Consent = st.Embedder
	}
	if err := r.c.RAG(proto.MethodRAGSetProject, proto.RAGProjectParams{Project: st.Project, Settings: set}, &st); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "%s: RAG %s\n", st.Project, words[0])
	return nil
}
