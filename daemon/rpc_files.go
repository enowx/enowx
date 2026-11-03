package daemon

import (
	"os"

	"github.com/enowx/enowx/core/files"
	"github.com/enowx/enowx/proto"
)

// fileHandlers read a session's folder, which can be large: they are
// slowMethods.
var fileHandlers = map[string]handler{
	proto.MethodFileList: with(func(c *conn, p proto.FileRef) (any, error) {
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		return files.List(dir, p.Path)
	}),
	proto.MethodFileChange: with(func(c *conn, p proto.FileChange) (any, error) {
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		changed, err := files.Change(dir, p)
		return proto.FileChanged{Path: changed}, err
	}),
	proto.MethodFileStat: with(func(c *conn, p proto.FileRef) (any, error) {
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		return files.Stat(dir, p.Path)
	}),
	proto.MethodFileUpload: with(func(c *conn, p proto.FileUpload) (any, error) {
		return c.srv.upload(p)
	}),
	proto.MethodFileFind: with(func(c *conn, p proto.FileFindParams) (any, error) {
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		limit := p.Limit
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		paths, err := files.Find(dir, p.Query, limit)
		return proto.FileFound{Paths: paths}, err
	}),
	proto.MethodFileBytes: with(func(c *conn, p proto.FileBytesParams) (any, error) {
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		return files.ReadPart(dir, p.Path, p.Abs, p.Offset)
	}),
	proto.MethodFileWrite: with(func(c *conn, p proto.FileWrite) (any, error) {
		if p.Abs {
			if _, err := c.srv.sessions.Get(p.Session); err != nil {
				return nil, err
			}
			return files.Write("", p)
		}
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		return files.Write(dir, p)
	}),
	proto.MethodFileRead: with(func(c *conn, p proto.FileRef) (any, error) {
		if p.Whole {
			dir := ""
			if !p.Abs {
				var err error
				if dir, err = c.srv.sessionDir(p.Session); err != nil {
					return nil, err
				}
			} else if _, err := c.srv.sessions.Get(p.Session); err != nil {
				return nil, err
			}
			return files.ReadWhole(dir, p.Path, p.Abs, p.Unless)
		}
		if p.Abs {
			if _, err := c.srv.sessions.Get(p.Session); err != nil {
				return nil, err
			}
			return files.ReadAbs(p.Path, p.Unless)
		}
		dir, err := c.srv.sessionDir(p.Session)
		if err != nil {
			return nil, err
		}
		return files.Read(dir, p.Path, p.Unless)
	}),
}

// sessionDir is the folder session id runs in: the home folder when it
// was given none.
func (s *Server) sessionDir(id string) (string, error) {
	sess, err := s.sessions.Get(id)
	if err != nil {
		return "", err
	}
	if dir := sess.Info().Dir; dir != "" {
		return dir, nil
	}
	return os.UserHomeDir()
}
