package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	readDefaultLimit = 2000
	readLineMost     = 2000
	readTextMost     = 100 << 10
	// The API takes a picture of up to 5 MB once base64: 3/4 of that as a file.
	readImageMost = 5 << 20 * 3 / 4
	sniffSize     = 8 << 10
)

const readSchema = `{"type":"object","properties":{"file_path":{"type":"string","description":"The file to read, absolute or relative to the working folder"},"offset":{"type":"integer","minimum":1,"description":"The line to start from, 1 being the first"},"limit":{"type":"integer","minimum":1,"description":"How many lines to read (default 2000)"}},"required":["file_path"],"additionalProperties":false}`

type readTool struct{}

func (readTool) Name() string { return "Read" }
func (readTool) Description() string {
	return "Read a file. Text comes with line numbers, up to 2000 lines or 100 KB at a time: use offset and limit for more. PNG, JPEG, GIF and WebP images come back as images. Binary files are refused."
}
func (readTool) Schema() json.RawMessage { return json.RawMessage(readSchema) }
func (readTool) ReadOnly() bool          { return true }
func (readTool) Edits() bool             { return false }

type readInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

func (readTool) Detail(input json.RawMessage) string {
	in, err := decode[readInput](input)
	if err != nil {
		return "Read"
	}
	if in.Offset > 1 {
		return oneLine(fmt.Sprintf("%s from line %d", in.FilePath, in.Offset), 200)
	}
	return oneLine(in.FilePath, 200)
}

func (readTool) ReadPaths(input json.RawMessage, dir string) []string {
	in, err := decode[readInput](input)
	if err != nil || in.FilePath == "" {
		return nil
	}
	return []string{ResolvePath(dir, in.FilePath)}
}

func (readTool) Run(ctx context.Context, input json.RawMessage, env Env) Result {
	in, err := decode[readInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if in.FilePath == "" {
		return failf("file_path is required")
	}
	abs := ResolvePath(env.Dir, in.FilePath)
	shown := shownPath(env.Dir, abs)
	info, err := os.Stat(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return failf("%s doesn't exist", shown)
	case err != nil:
		return failf("read %s: %v", shown, err)
	case info.IsDir():
		return failf("%s is a folder: list it with Glob", shown)
	case !info.Mode().IsRegular():
		// Reading a pipe or a device could block forever.
		return failf("%s isn't a regular file", shown)
	}
	f, err := os.Open(abs)
	if err != nil {
		return failf("read %s: %v", shown, err)
	}
	defer f.Close()
	head := make([]byte, sniffSize)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return failf("read %s: %v", shown, err)
	}
	head = head[:n]
	if media := imageType(head); media != "" {
		return readImage(f, shown, media, info.Size())
	}
	if bytes.HasPrefix(head, []byte("%PDF-")) || strings.EqualFold(filepath.Ext(abs), ".pdf") {
		return Result{Text: fmt.Sprintf("%s is a PDF of %d bytes, which this tool can't show: extract its text with a program through Bash (pdftotext, for one).", shown, info.Size())}
	}
	if looksBinary(head) {
		return failf("%s is a binary file of %d bytes, not shown", shown, info.Size())
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return failf("read %s: %v", shown, err)
	}
	return readLines(ctx, f, shown, in.Offset, in.Limit)
}

// readImage returns the whole file as a picture for the model.
func readImage(f *os.File, shown, media string, size int64) Result {
	if size > readImageMost {
		return failf("%s is an image of %d bytes, over the %d bytes the API takes", shown, size, readImageMost)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return failf("read %s: %v", shown, err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return failf("read %s: %v", shown, err)
	}
	return Result{
		Text:   fmt.Sprintf("%s: %s, %d bytes", shown, media, len(data)),
		Images: []Image{{MediaType: media, Base64: base64.StdEncoding.EncodeToString(data)}},
	}
}

// readLines streams the lines asked for, so a huge file costs only what is shown.
func readLines(ctx context.Context, r io.Reader, shown string, offset, limit int) Result {
	if offset < 1 {
		offset = 1
	}
	if limit < 1 {
		limit = readDefaultLimit
	}
	br := bufio.NewReaderSize(r, 64<<10)
	var out strings.Builder
	number, shownLines := 0, 0
	for {
		line, clipped, err := readLine(br)
		if err == io.EOF {
			break
		}
		if err != nil {
			return failf("read %s: %v", shown, err)
		}
		number++
		if number%1024 == 0 && ctx.Err() != nil {
			return failf("interrupted")
		}
		if number < offset {
			continue
		}
		if shownLines == limit || out.Len() >= readTextMost {
			fmt.Fprintf(&out, "[lines %d-%d shown, the file goes on: continue with offset %d]\n", offset, number-1, number)
			return Result{Text: out.String()}
		}
		text := strings.ToValidUTF8(string(line), "\uFFFD")
		if clipped {
			text += "… [line clipped]"
		}
		fmt.Fprintf(&out, "%6d\t%s\n", number, text)
		shownLines++
	}
	switch {
	case number == 0:
		return Result{Text: shown + " is empty"}
	case shownLines == 0:
		return failf("%s has %d lines, offset %d is past its end", shown, number, offset)
	}
	return Result{Text: out.String()}
}

// readLine is the next line without its ending, cut at readLineMost, however long it
// runs; io.EOF when no line is left.
func readLine(br *bufio.Reader) (line []byte, clipped bool, err error) {
	for {
		part, err := br.ReadSlice('\n')
		if room := readLineMost - len(line); room >= len(part) {
			line = append(line, part...)
		} else {
			line = append(line, part[:max(room, 0)]...)
			clipped = true
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			err = nil
		case err != nil:
			return nil, false, err
		}
		return bytes.TrimRight(line, "\r\n"), clipped, nil
	}
}

// imageType is the media type of a picture the API takes, from its first bytes.
func imageType(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return "image/gif"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

// looksBinary says head isn't text: it holds a NUL, or too much that isn't characters.
func looksBinary(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}
	bad, total := 0, 0
	for len(head) > 0 {
		r, size := utf8.DecodeRune(head)
		head = head[size:]
		total++
		if r == utf8.RuneError && size == 1 || r < 32 && r != '\t' && r != '\n' && r != '\r' && r != '\f' && r != 27 {
			bad++
		}
	}
	return bad*10 > total*3
}
