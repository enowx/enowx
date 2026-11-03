package client

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/enowx/enowx/internal/hostpath"
	"github.com/enowx/enowx/proto"
)

// ListFiles lists the folder path of session id's folder ("" for itself).
func (c *Client) ListFiles(id, path string) (proto.FileListing, error) {
	var l proto.FileListing
	err := c.Call(proto.MethodFileList, proto.FileRef{Session: id, Path: path}, &l)
	return l, err
}

// ReadFile reads the file path of session id's folder, or at an absolute
// path ("/tmp/shot.png") anywhere on the host; when it last changed at
// unless (set), the answer only says Unchanged.
func (c *Client) ReadFile(id, path string, unless time.Time) (proto.FileContent, error) {
	var f proto.FileContent
	ref := proto.FileRef{Session: id, Path: path, Unless: unless, Abs: hostpath.IsAbs(path)}
	err := c.Call(proto.MethodFileRead, ref, &f)
	return f, err
}

// ReadFileWhole is ReadFile up to proto.MaxEdit, to edit it.
func (c *Client) ReadFileWhole(id, path string, unless time.Time) (proto.FileContent, error) {
	var f proto.FileContent
	ref := proto.FileRef{Session: id, Path: path, Unless: unless, Abs: hostpath.IsAbs(path), Whole: true}
	err := c.Call(proto.MethodFileRead, ref, &f)
	return f, err
}

// WriteFile saves the file path of session id's folder as ReadFile found it: Force writes
// over what changed since unless.
func (c *Client) WriteFile(id, path, text string, unless time.Time, force bool) (proto.FileWritten, error) {
	var out proto.FileWritten
	err := c.Call(proto.MethodFileWrite, proto.FileWrite{Session: id, Path: path, Abs: hostpath.IsAbs(path), Text: text, Unless: unless, Force: force}, &out)
	return out, err
}

// FindFiles finds up to limit files of session id's folder by a few
// letters of their path, best first.
func (c *Client) FindFiles(id, query string, limit int) ([]string, error) {
	var out proto.FileFound
	err := c.Call(proto.MethodFileFind, proto.FileFindParams{Session: id, Query: query, Limit: limit}, &out)
	return out.Paths, err
}

// ReadBytes reads the whole file path of session id's folder (an absolute
// path on the host when abs), as it is, up to limit bytes. Daemons before
// file.bytes answer with what file.read sends: text up to a megabyte, and
// images.
func (c *Client) ReadBytes(id, path string, abs bool, limit int64) ([]byte, error) {
	var data []byte
	for {
		var part proto.FileBytes
		err := c.Call(proto.MethodFileBytes, proto.FileBytesParams{Session: id, Path: path, Abs: abs, Offset: int64(len(data))}, &part)
		if IsUnknownMethod(err) {
			f, err := c.ReadFile(id, path, time.Time{})
			if f.Data != nil {
				return f.Data, err
			}
			return []byte(f.Text), err
		}
		if err != nil {
			return nil, err
		}
		if part.Size > limit {
			return nil, fmt.Errorf("client: %s is larger than %d bytes", path, limit)
		}
		data = append(data, part.Data...)
		if len(part.Data) == 0 || int64(len(data)) >= part.Size {
			return data, nil
		}
	}
}

// Ports lists the TCP ports session id's processes listen on.
func (c *Client) Ports(id string) (proto.SessionPorts, error) {
	var p proto.SessionPorts
	err := c.Call(proto.MethodSessionPorts, proto.SessionRef{ID: id}, &p)
	return p, err
}

// Upload puts the local file at path on session id's host, in parts, and
// returns where it is there.
func (c *Client) Upload(id, path string) (string, error) { return c.upload(id, path, "") }

// upload sends the file at path to session id's host: with its uploads,
// or into folder into of its folder.
func (c *Client) upload(id, path, into string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, proto.UploadPart)
	remote := ""
	for {
		n, err := io.ReadFull(f, buf)
		if n > 0 || remote == "" {
			var out proto.FileUploaded
			if cerr := c.Call(proto.MethodFileUpload, proto.FileUpload{Session: id, Name: filepath.Base(path), Path: remote, Data: buf[:n], Into: into}, &out); cerr != nil {
				return "", cerr
			}
			if into != "" && !out.Placed {
				return "", ErrOlderDaemon
			}
			remote = out.Path
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return remote, nil
		}
		if err != nil {
			return "", err
		}
	}
}

// ChangeFile makes a change to path of session id's folder (proto.FileNewFile
// and the others) and returns the path it made or renamed to.
func (c *Client) ChangeFile(id, change, path, name string) (string, error) {
	var out proto.FileChanged
	err := c.Call(proto.MethodFileChange, proto.FileChange{Session: id, Change: change, Path: path, Name: name}, &out)
	return out.Path, err
}

// StatFile says what path of session id's folder is.
func (c *Client) StatFile(id, path string) (proto.FileStat, error) {
	var out proto.FileStat
	err := c.Call(proto.MethodFileStat, proto.FileRef{Session: id, Path: path}, &out)
	return out, err
}

// ErrOlderDaemon says the host's daemon predates what was asked of it.
var ErrOlderDaemon = errors.New("the daemon on this host is older and can't do that yet: reconnect to update it")

// AddFiles copies files and folders of this machine into folder (a path of
// session id's folder, "" for the folder itself) on its host, a folder
// with all it holds; a name taken there gets a number. each, when set, is
// told of every file copied.
func (c *Client) AddFiles(id string, local []string, folder string, each func(path string)) error {
	if folder == "" {
		folder = "."
	}
	for _, top := range local {
		info, err := os.Stat(top)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if _, err := c.upload(id, top, folder); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(top), err)
			}
			if each != nil {
				each(top)
			}
			continue
		}
		base := filepath.Dir(top)
		err = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err // links and the like stay behind
			}
			rel, err := filepath.Rel(base, filepath.Dir(p))
			if err != nil {
				return err
			}
			if _, err := c.upload(id, p, path.Join(folder, filepath.ToSlash(rel))); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(p), err)
			}
			if each != nil {
				each(p)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
