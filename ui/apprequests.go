package ui

import (
	"encoding/json"
	"errors"
	"path"
	"strings"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
)

// linkEvent applies an event of links, auto pushes or a request for the
// app from a host's connection, conn telling it and its daemon then; nil
// for other events. It runs off the main thread; what it returns runs on
// it.
func (a *App) linkEvent(ev proto.Event, conn func() (*client.Client, string)) func() {
	switch ev.Name {
	case proto.EventProjectLinksChanged:
		var list []proto.ProjectLinkState
		if json.Unmarshal(ev.Data, &list) != nil {
			return nil
		}
		return func() { a.mergeLinks(list) }
	case proto.EventProjectFilesChanged:
		var ref proto.ProjectRef
		if json.Unmarshal(ev.Data, &ref) != nil {
			return nil
		}
		return func() {
			if _, id := conn(); id != "" {
				a.filesChanged(id, ref.Path)
			}
		}
	case proto.EventAutopush:
		var st proto.AutopushStatus
		if json.Unmarshal(ev.Data, &st) != nil {
			return nil
		}
		return func() { a.pushChanged(st) }
	case proto.EventProjectHistory:
		var e proto.ProjectHistoryEntry
		if json.Unmarshal(ev.Data, &e) != nil {
			return nil
		}
		return func() {
			if f := a.linksForm; f != nil && f.id == e.Project {
				f.history = append([]proto.ProjectHistoryEntry{e}, f.history...)
			}
		}
	case proto.EventAppRequest:
		var req proto.AppRequest
		if json.Unmarshal(ev.Data, &req) != nil {
			return nil
		}
		return func() {
			if cl, id := conn(); cl != nil {
				a.appRequest(cl, id, req)
			}
		}
	}
	return nil
}

func (a *App) pushChanged(st proto.AutopushStatus) {
	if a.links.pushes == nil {
		a.links.pushes = map[string]proto.AutopushStatus{}
	}
	a.links.pushes[st.Project] = st
	if st.Err != "" && st.On {
		a.notify("Autopush of “" + st.Name + "”: " + st.Err)
	}
}

// appRequest does what an agent on the host of cl (daemon id) asked
// across hosts, and answers it.
func (a *App) appRequest(cl *client.Client, id string, req proto.AppRequest) {
	reply := func(result any, err error) {
		rep := proto.AppReply{ID: req.ID}
		if err != nil {
			rep.Error = client.Explain(err)
		} else {
			rep.Result, _ = json.Marshal(result)
		}
		go func() { _ = cl.ReplyToApp(rep) }()
	}
	switch req.Kind {
	case "project.copy":
		var p proto.ProjectCopyParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, err)
			return
		}
		src, ok := a.reach(id)
		if !ok {
			reply(nil, errors.New("the app lost its connection to this host"))
			return
		}
		target, ok := a.hostNamed(p.To)
		if !ok {
			reply(nil, errors.New("no host “"+p.To+"” in the app's list: "+strings.Join(a.hostNames(), ", ")))
			return
		}
		if target.ID == src.host.ID {
			reply(nil, errors.New("the project is on "+target.Name+" already"))
			return
		}
		go func() {
			res, err := a.copyProjectTo(src, target, p)
			reply(res, err)
		}()
	case "project.sync":
		var ref proto.ProjectLinkRef
		if err := json.Unmarshal(req.Params, &ref); err != nil {
			reply(nil, err)
			return
		}
		a.syncLinkNow(ref.ID, func(st proto.ProjectLinkState, err error) { reply(st, err) })
	case "session.hibernate":
		a.hibernateAsked(cl, req, reply)
	case "project.context", "project.read":
		a.projectAsk(req, reply)
	case "sessions.everywhere":
		a.sessionsEverywhere(id, reply)
	case "session.peek":
		var p proto.SessionPeekParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, err)
			return
		}
		a.peekSession(p, reply)
	case "host.call":
		a.hostCallAsked(id, req, reply)
	case "app.settings":
		a.appSettingsAsked(req, reply)
	default:
		reply(nil, errors.New("this app doesn't know “"+req.Kind+"”: update it"))
	}
}

// syncLinkNow syncs link id now when both its hosts are reached.
func (a *App) syncLinkNow(id string, done func(proto.ProjectLinkState, error)) {
	l, ok := a.links.links[id]
	if !ok {
		done(proto.ProjectLinkState{}, errors.New("no link "+id+": the app doesn't know it yet"))
		return
	}
	src, ok1 := a.reach(l.Source.Daemon)
	dst, ok2 := a.reach(l.Target.Daemon)
	if !ok1 || !ok2 {
		done(proto.ProjectLinkState{}, errors.New("both "+l.Source.Host+" and "+l.Target.Host+" must be online to sync"))
		return
	}
	a.runLink(l.ProjectLink, src, dst, done)
}

// hostNamed is the host of the app's list named (or with ID) name.
func (a *App) hostNamed(name string) (hosts.Host, bool) {
	for _, h := range a.allHosts() {
		if h.ID == name || strings.EqualFold(h.Name, name) {
			return h, true
		}
	}
	return hosts.Host{}, false
}

func (a *App) hostNames() []string {
	var out []string
	for _, h := range a.allHosts() {
		out = append(out, h.Name)
	}
	return out
}

// copyProjectTo copies a project on src's host to target, without its
// sessions, and links both copies; it runs off the main thread.
func (a *App) copyProjectTo(src hostConn, target hosts.Host, p proto.ProjectCopyParams) (proto.ProjectCopyResult, error) {
	var dst *client.Client
	done := make(chan struct{})
	var opts hostconn.Options
	a.onMain(func() {
		if h, ok := a.reachHost(target.ID); ok {
			dst = h.cl
		}
		opts = a.hostOptions(a.gen)
		close(done)
	})
	<-done
	if dst == nil {
		c, err := hostconn.Connect(target, opts)
		if err != nil {
			return proto.ProjectCopyResult{}, err
		}
		defer c.Close()
		dst = c
	}
	dest := p.Path
	if dest == "" {
		dest = homeRelative(src.info.Home, path.Dir(p.Project)) + "/" + path.Base(p.Project)
	}
	info, err := src.cl.PackTransfer(proto.TransferPackParams{Project: p.Project, Sessions: []string{}, Env: p.Env})
	if err != nil {
		return proto.ProjectCopyResult{}, err
	}
	defer func() { _ = src.cl.DiscardTransfer(info.ID) }()
	if err := client.CopyTransfer(src.cl, dst, info.ID, info.Size, nil); err != nil {
		_ = dst.DiscardTransfer(info.ID)
		return proto.ProjectCopyResult{}, err
	}
	res, err := dst.UnpackTransfer(info.ID, dest)
	if err != nil {
		_ = dst.DiscardTransfer(info.ID)
		return proto.ProjectCopyResult{}, err
	}
	l, err := recordCopy(src.cl, src.host.Name, p.Project, dst, target.Name, res.Project.Path)
	if err != nil {
		return proto.ProjectCopyResult{Path: res.Project.Path}, err
	}
	a.onMain(func() { a.mergeLinks([]proto.ProjectLinkState{{ProjectLink: l}}) })
	return proto.ProjectCopyResult{Path: res.Project.Path, Link: l}, nil
}

// reachHost is host id of the app's list, when reached now.
func (a *App) reachHost(id string) (hostConn, bool) {
	for _, h := range a.reachable() {
		if h.host.ID == id {
			return h, true
		}
	}
	return hostConn{}, false
}

// homeRelative spells dir from the home folder home ("~/..."), or as it
// is outside it.
func homeRelative(home, dir string) string {
	home = strings.TrimSuffix(home, "/")
	if home != "" {
		if rest, ok := strings.CutPrefix(dir, home); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
			return "~" + rest
		}
	}
	return dir
}

// recordCopy links the project at srcPath on src's host, the source, to
// its copy at dstPath on dst's: the copy takes the source's ID, and both
// hosts keep the link. It runs off the main thread.
func recordCopy(src *client.Client, srcName, srcPath string, dst *client.Client, dstName, dstPath string) (proto.ProjectLink, error) {
	id, err := src.ProjectID(srcPath)
	if err != nil {
		return proto.ProjectLink{}, err
	}
	si, err := src.Info()
	if err != nil {
		return proto.ProjectLink{}, err
	}
	di, err := dst.Info()
	if err != nil {
		return proto.ProjectLink{}, err
	}
	if si.ID == "" || di.ID == "" {
		return proto.ProjectLink{}, errors.New("a host's daemon is too old to remember copies: update it")
	}
	l, err := src.SaveProjectLink(proto.ProjectLink{
		Project: id.ID, Name: path.Base(id.Path), Direction: proto.SyncTwoWay,
		Source: proto.ProjectEnd{Daemon: si.ID, Host: srcName, Path: id.Path},
		Target: proto.ProjectEnd{Daemon: di.ID, Host: dstName, Path: dstPath},
	})
	if err != nil {
		return l, err
	}
	_, err = dst.SaveProjectLink(l)
	return l, err
}
