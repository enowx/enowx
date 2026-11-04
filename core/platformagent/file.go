package platformagent

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/enowx/enowx/core/platformagent/record"
)

// convFile is the conversation's file, one record a line. Its lock is held for the write
// alone; records come from a turn's side-by-side tools too.
type convFile struct {
	mu   sync.Mutex
	f    *os.File
	path string
	err  error // the first write that failed
}

func openConvFile(path string) (*convFile, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("platformagent: conversation file: %w", err)
	}
	// A crash can leave half a line; the next record mustn't join it.
	if info, err := f.Stat(); err == nil && info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			_, _ = f.Write([]byte("\n"))
		}
	}
	return &convFile{f: f, path: path}, nil
}

func (c *convFile) write(line []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f == nil {
		return
	}
	if _, err := c.f.Write(append(line, '\n')); err != nil && c.err == nil {
		c.err = err
	}
}

func (c *convFile) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *convFile) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f == nil {
		return nil
	}
	err := c.f.Close()
	c.f = nil
	return err
}

// emit keeps r in the file and shows it. It may come from any of a turn's goroutines.
func (a *Agent) emit(r record.Record) {
	if r.Time.IsZero() {
		r.Time = time.Now().UTC()
	}
	line := marshalStable(r)
	held := false
	for _, secret := range a.secrets {
		held = held || bytes.Contains(line, []byte(secret))
		line = bytes.ReplaceAll(line, []byte(secret), []byte("[key]"))
	}
	if held {
		// The screen gets what the file gets.
		var clean record.Record
		if json.Unmarshal(line, &clean) != nil {
			clean = record.Record{Type: record.Notice, Level: "warning", Text: "A record held the API key, so it isn't shown.", Time: r.Time}
		}
		r = clean
	}
	a.mu.Lock()
	file := a.file
	a.mu.Unlock()
	if file != nil {
		file.write(line)
	}
	a.front.Event(r)
}

func (a *Agent) notice(level, text string) {
	a.emit(record.Record{Type: record.Notice, Level: level, Text: a.scrub(text)})
}

func (a *Agent) fileFailure() error {
	a.mu.Lock()
	file := a.file
	a.mu.Unlock()
	if file == nil {
		return nil
	}
	if err := file.failure(); err != nil {
		return fmt.Errorf("platformagent: keeping the conversation: %w", err)
	}
	return nil
}

// writeMeta records the settings now.
func (a *Agent) writeMeta() {
	a.mu.Lock()
	r := record.Record{
		Type:         record.Meta,
		Conversation: a.conv,
		Dir:          a.opts.Dir,
		Model:        a.model,
		Effort:       a.effort,
		Mode:         a.mode,
		Budget:       a.budget,
		Window:       specOf(a.model).window,
	}
	a.mu.Unlock()
	a.emit(r)
}

func newConversationID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("platformagent: conversation ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func conversationPath(dir, id string) string { return filepath.Join(dir, id+".jsonl") }
