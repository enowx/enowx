package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/enowx/enowx/core/files"
	"github.com/enowx/enowx/core/notesio"
	"github.com/enowx/enowx/proto"
)

// Imports of notes run as jobs the app and agents follow, one writing at
// a time; files of the app's machine are staged in <daemon dir>/
// notes-staging first, and exports made for the app wait there to be
// read, a day at most.

type noteImports struct {
	mu    sync.Mutex
	list  []*importJob // oldest first; the last few done are kept
	write sync.Mutex   // one import writes notes at a time
}

type importJob struct {
	status proto.NoteImportStatus // guarded by noteImports.mu
	cancel context.CancelFunc
}

// stagedFor is how long a staged file or an export waits to be used.
const stagedFor = 24 * time.Hour

func (s *Server) stagingDir() string {
	return filepath.Join(filepath.Dir(s.WorktreeRoot), "notes-staging")
}

// hostPath is p on this host: "~" its user's home, made absolute.
func hostPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%q isn't a full path on this host", p)
	}
	return filepath.Clean(p), nil
}

func randomID() string {
	var b [6]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// sweepStaging forgets what was staged more than stagedFor ago.
func (s *Server) sweepStaging() {
	entries, err := os.ReadDir(s.stagingDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > stagedFor {
			os.RemoveAll(filepath.Join(s.stagingDir(), e.Name()))
		}
	}
}

// stage writes a part of a file staged for an import.
func (s *Server) stage(p proto.NoteStage) (proto.NoteStaged, error) {
	path := p.Path
	flags := os.O_WRONLY | os.O_APPEND
	if path == "" {
		s.sweepStaging()
		name := filepath.Base(strings.ReplaceAll(p.Name, `\`, "/"))
		if name == "." || name == "/" || name == "" {
			return proto.NoteStaged{}, errors.New("stage: the file has no name")
		}
		dir := filepath.Join(s.stagingDir(), randomID())
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return proto.NoteStaged{}, fmt.Errorf("stage: %w", err)
		}
		path, flags = filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL
	} else if filepath.Dir(filepath.Dir(path)) != s.stagingDir() {
		return proto.NoteStaged{}, fmt.Errorf("stage: %s isn't where the file was going", path)
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return proto.NoteStaged{}, fmt.Errorf("stage: %w", err)
	}
	if _, err := f.Write(p.Data); err != nil {
		f.Close()
		return proto.NoteStaged{}, fmt.Errorf("stage: %w", err)
	}
	return proto.NoteStaged{Path: path}, f.Close()
}

// startNoteImport starts importing p, returning its first status.
func (s *Server) startNoteImport(p proto.NoteImportParams) (proto.NoteImportStatus, error) {
	path := ""
	if p.From != proto.ImportNotionAPI {
		var err error
		if path, err = hostPath(p.Path); err != nil {
			return proto.NoteImportStatus{}, err
		}
		if _, err := os.Stat(path); err != nil {
			return proto.NoteImportStatus{}, fmt.Errorf("import: %w", err)
		}
	} else if p.Token == "" {
		return proto.NoteImportStatus{}, errors.New("import: Notion's API needs an integration's token")
	}
	switch p.Mode {
	case "", proto.ImportSkip, proto.ImportOverwrite, proto.ImportDuplicate:
	default:
		return proto.NoteImportStatus{}, fmt.Errorf("import: no mode %q (skip, overwrite, duplicate)", p.Mode)
	}
	source := filepath.Base(path)
	if path == "" {
		source = "Notion"
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &importJob{cancel: cancel, status: proto.NoteImportStatus{
		ID: randomID(), Source: source, From: p.From, DryRun: p.DryRun, State: proto.ImportRunning, Phase: "Reading", Started: time.Now().UTC(),
	}}
	s.noteIO.mu.Lock()
	s.noteIO.list = append(s.noteIO.list, job)
	first := job.status
	s.noteIO.mu.Unlock()
	s.broadcast(proto.EventNoteImport, first)
	go s.runImport(ctx, job, path, p)
	return first, nil
}

// update changes job's status and tells the apps, at most a few times a
// second unless forced.
func (s *Server) importUpdate(job *importJob, force bool, change func(st *proto.NoteImportStatus)) {
	s.noteIO.mu.Lock()
	change(&job.status)
	now := job.status
	s.noteIO.mu.Unlock()
	if force || now.Done == now.Total || now.Done%10 == 0 {
		s.broadcast(proto.EventNoteImport, now)
	}
}

func (s *Server) runImport(ctx context.Context, job *importJob, path string, p proto.NoteImportParams) {
	defer job.cancel()
	staged := path != "" && strings.HasPrefix(path, s.stagingDir()+string(filepath.Separator))
	b, err := notesio.Read(ctx, path, notesio.Options{From: p.From, RowsAsNotes: p.RowsAsNotes, Token: p.Token})
	var report proto.NoteImportReport
	if err == nil {
		o := notesio.ApplyOptions{Group: strings.TrimSpace(p.Group), Project: p.Project, Mode: p.Mode}
		var pv proto.NoteImportPreview
		if pv, err = b.Preview(s.notes, o); err == nil {
			s.importUpdate(job, true, func(st *proto.NoteImportStatus) {
				st.From, st.Preview, st.Total = b.From, &pv, len(b.Docs)
				if !p.DryRun {
					st.Phase = "Writing"
				}
			})
		}
		if err == nil && !p.DryRun {
			o.Progress = func(done, total int) {
				s.importUpdate(job, false, func(st *proto.NoteImportStatus) { st.Done, st.Total = done, total })
			}
			s.noteIO.write.Lock()
			report, err = b.Apply(ctx, s.notes, o)
			s.noteIO.write.Unlock()
			if err == nil && p.Project != "" {
				err = s.attachImported(report.Notes, p.Project)
			}
			s.groupsChanged(nil)
		}
	}
	if staged && !p.DryRun {
		os.RemoveAll(filepath.Dir(path)) // the app staged it for this import
	}
	s.importUpdate(job, true, func(st *proto.NoteImportStatus) {
		switch {
		case errors.Is(err, context.Canceled):
			st.State, st.Error = proto.ImportCanceled, ""
		case err != nil:
			st.State, st.Error = proto.ImportFailed, err.Error()
		default:
			st.State, st.Phase = proto.ImportDone, ""
			if !p.DryRun {
				st.Report = &report
			}
		}
	})
	s.noteIO.mu.Lock()
	s.noteIO.trim()
	s.noteIO.mu.Unlock()
}

// trim forgets the oldest finished imports past keepDone; mu is held.
func (n *noteImports) trim() {
	done := 0
	for i := len(n.list) - 1; i >= 0; i-- {
		if n.list[i].status.State == proto.ImportRunning {
			continue
		}
		if done++; done > keepDone {
			n.list = append(n.list[:i], n.list[i+1:]...)
		}
	}
}

func (s *Server) noteImport(id string) (*importJob, error) {
	s.noteIO.mu.Lock()
	defer s.noteIO.mu.Unlock()
	for _, j := range s.noteIO.list {
		if j.status.ID == id {
			return j, nil
		}
	}
	return nil, fmt.Errorf("no import %q", id)
}

func (s *Server) noteImportList() []proto.NoteImportStatus {
	s.noteIO.mu.Lock()
	defer s.noteIO.mu.Unlock()
	out := []proto.NoteImportStatus{}
	for _, j := range s.noteIO.list {
		out = append(out, j.status)
	}
	return out
}

// stopAll cancels the imports running.
func (n *noteImports) stopAll() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, j := range n.list {
		j.cancel()
	}
}

// exportNotes writes the notes p picks to p.Out, or for the app to read.
func (s *Server) exportNotes(p proto.NoteExportParams) (proto.NoteExported, error) {
	if p.Format == proto.ExportRepo {
		return s.exportRepo(p)
	}
	sel := notesio.Selection{IDs: p.IDs, Group: p.Group, All: p.All}
	if len(p.IDs) == 0 && p.Group == "" && !p.All {
		return proto.NoteExported{}, errors.New("export: pick notes, a group, or all")
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.WorktreeRoot), ".export-*")
	if err != nil {
		return proto.NoteExported{}, fmt.Errorf("export: %w", err)
	}
	defer os.Remove(tmp.Name())
	name, count, err := notesio.Export(s.notes, sel, p.Format, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return proto.NoteExported{}, err
	}
	var out string
	if p.Out == "" {
		s.sweepStaging()
		dir := filepath.Join(s.stagingDir(), randomID())
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return proto.NoteExported{}, fmt.Errorf("export: %w", err)
		}
		out = filepath.Join(dir, name)
	} else {
		if out, err = hostPath(p.Out); err != nil {
			return proto.NoteExported{}, err
		}
		if info, err := os.Stat(out); err == nil && info.IsDir() {
			out = filepath.Join(out, name)
		}
	}
	if err := os.Rename(tmp.Name(), out); err != nil {
		data, rerr := os.ReadFile(tmp.Name())
		if rerr != nil {
			return proto.NoteExported{}, fmt.Errorf("export: %w", err)
		}
		if err := os.WriteFile(out, data, 0o600); err != nil {
			return proto.NoteExported{}, fmt.Errorf("export: %w", err)
		}
	}
	info, err := os.Stat(out)
	if err != nil {
		return proto.NoteExported{}, fmt.Errorf("export: %w", err)
	}
	return proto.NoteExported{Path: out, Name: name, Size: info.Size(), Notes: count}, nil
}

// exportBytes reads a part of an export made for the app.
func (s *Server) exportBytes(p proto.NoteExportBytesParams) (proto.FileBytes, error) {
	rel, err := filepath.Rel(s.stagingDir(), p.Path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return proto.FileBytes{}, fmt.Errorf("export: %s isn't an export", p.Path)
	}
	return files.ReadPart(s.stagingDir(), rel, false, p.Offset)
}
