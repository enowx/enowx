package ui

import (
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/projectsync"
	"github.com/enowx/enowx/proto"
)

// A project's dialogs to and with other hosts, with what they'd have read from
// a host that isn't here, for the layout audit.

// auditLinks gives project p a copy on another host, conflicts and all.
func auditLinks(a *App) *linksForm {
	now := time.Now()
	a.daemonInfo.ID = "here"
	l := proto.ProjectLinkState{ProjectLink: proto.ProjectLink{ID: "l1", Project: "pid", Name: "antares", Direction: proto.SyncTwoWay, Auto: true, Interval: 300,
		Source: proto.ProjectEnd{Daemon: "here", Host: "this Mac", Path: "/Users/enowdev/Project/antares"},
		Target: proto.ProjectEnd{Daemon: "labs", Host: "a host with a rather long name to run", Path: "/home/enowdev/code/antares/and/a/long/way/down/to/have/to/run/in/its/row"}},
		Status: proto.ProjectLinkStatus{Last: now.Add(-time.Hour), Err: "the host didn't answer, and more of why so that the line has to wrap",
			Conflicts: []proto.ProjectSyncConflict{
				{Path: "src/a/file/with/a/long/path/that/has/to/run/inside/its/row/of/the/card.go", Found: now, Sides: [2]proto.SyncFile{{Size: 1200, Modified: now}, {Size: 900, Modified: now}}},
				{Path: "gone.txt", Found: now, Gone: [2]bool{false, true}, Sides: [2]proto.SyncFile{{Size: 5, Modified: now}, {}}},
			}}}
	a.links.links = map[string]proto.ProjectLinkState{"l1": l}
	a.links.running = map[string]string{}
	f := &linksForm{open: true, project: a.projects[0], id: "pid", busy: map[string]string{}, pushRead: true,
		history: []proto.ProjectHistoryEntry{
			{Time: now, Kind: "sync", OK: true, Summary: "3 files each way"},
			{Time: now.Add(-time.Hour), Kind: "push", Summary: "a push", Err: "rejected: a reason long enough to wrap inside its row of the history tab, whatever the width of the dialog"},
		}}
	f.push = proto.AutopushStatus{AutopushSettings: proto.AutopushSettings{Project: "pid", Name: "antares", On: true, Host: "here", Branch: "enowx/autosync", Remote: "origin", PR: true}, Err: "the last push failed for a reason", Visibility: "public", LastPush: now, Commit: "abcdef123", Files: 3, Next: now.Add(time.Hour), Here: true, BranchURL: "https://example.com", PRURL: "https://example.com/pr"}
	f.pushText = pushFieldsOf(f.push)
	a.linksForm, a.page = f, ""
	return f
}

func projectAuditPages() []auditPage {
	other := hosts.Host{ID: "labs", Name: "enowxlabs", Address: "root@enowxlabs"}
	withHost := func(a *App) {
		a.hostList = []hosts.Host{other}
	}
	plan := projectsync.Plan{
		Files: []projectsync.File{{Path: "README.md"}, {Path: "a/b.go", From: projectsync.There}, {Path: "a/deleted.go", Delete: true}, {Path: "a/both/with/a/name/too/long/for/its/row/so/that/it/has/to/run/in/it.go", Conflict: true}},
		Sessions: []projectsync.Session{{Name: "Fix it", New: true}, {Name: "Chat", Restarts: true, From: projectsync.There}, {Name: "Both", Key: "k", Conflict: true}},
		Branches: []projectsync.Branch{{Name: "main", Commit: "abcdef123"}},
	}
	return []auditPage{
		{name: "project-clone-running", open: func(a *App) {
			a.page = ""
			a.openClone()
			a.clone.url, a.clone.name, a.clone.dir = "https://github.com/enowx/enowx.git", "enowx", "~/code"
			a.clone.job = &proto.CloneStatus{State: proto.CloneRunning, Phase: "Receiving objects", Percent: 42, Line: "remote: a line of git's output, long enough that it has to run inside the row of the dialog"}
		}},
		{name: "project-clone-errors", open: func(a *App) {
			a.page = ""
			a.openClone()
			a.clone.url, a.clone.dir = "not a url", "~/code"
			a.clone.err, a.clone.name, a.clone.named = "Stopped; nothing was left behind.", "enowx", true
			a.clone.taken, a.clone.checked = "~/code/enowx", "~/code/enowx"
		}},
		{name: "project-clone-lately", open: func(a *App) {
			a.page = ""
			a.openClone()
			a.clone.recent = []string{"https://github.com/enowx/enowx.git", "https://github.com/enowx/enowx-web.git"}
		}},
		{name: "project-clone-to", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.cloningTo = &cloneToForm{open: true, project: a.projects[0], dir: "~/code", target: other, remote: "https://github.com/enowx/enowx.git", running: true, status: proto.CloneStatus{Phase: "Receiving objects", Percent: 42}}
		}},
		{name: "project-clone-to-none", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.cloningTo = &cloneToForm{open: true, project: a.projects[0], dir: "~/code", target: other, err: "antares has no Git remote to clone from: push it somewhere first, or copy it to the other host"}
		}},
		{name: "project-clone-to-done", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.cloningTo = &cloneToForm{open: true, project: a.projects[0], dir: "~/code", target: other, remote: "https://github.com/enowx/enowx.git", status: proto.CloneStatus{State: proto.CloneDone, Path: "~/code/enowx"}}
		}},
		{name: "project-copy", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.openCopy(a.projects[0])
			a.copying.target, a.copying.env = other, true
			a.copying.err = "the copy didn't go, and a reason long enough to wrap inside its notice"
		}},
		{name: "project-copy-none", open: func(a *App) {
			a.hostList, a.page = nil, ""
			a.openCopy(a.projects[0])
		}},
		{name: "project-copy-carrying", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.openCopy(a.projects[0])
			a.copying.target, a.copying.step, a.copying.sent, a.copying.total = other, copyCarrying, 1<<20, 8<<20
		}},
		{name: "project-copy-done", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.openCopy(a.projects[0])
			a.copying.target, a.copying.step = other, copyDone
			a.copying.result = proto.TransferResult{Project: proto.ProjectInfo{Path: "/home/enowdev/antares"}, Notes: []string{"Not linked to this copy for syncs: a reason"}}
		}},
		{name: "project-sync-pick", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.projectSyncing = &projectSyncForm{open: true, project: a.projects[0], target: other, folder: "~/code/antares", env: true, err: "the host didn't answer, and a reason long enough to wrap inside its notice"}
		}},
		{name: "project-sync-comparing", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.projectSyncing = &projectSyncForm{open: true, project: a.projects[0], target: other, folder: "~/code/antares", step: syncComparing, doing: "Tailscale wants you to check in a browser, and a line long enough to run", signIn: "https://login.tailscale.com/a/b"}
		}},
		{name: "project-sync-review", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.projectSyncing = &projectSyncForm{open: true, project: a.projects[0], target: other, folder: "~/code/antares", step: syncReview, plan: plan}
		}},
		{name: "project-sync-same", open: func(a *App) {
			withHost(a)
			a.page = ""
			a.projectSyncing = &projectSyncForm{open: true, project: a.projects[0], target: other, folder: "~/code/antares", step: syncReview}
		}},
		{name: "project-sync-done", open: func(a *App) {
			withHost(a)
			a.page = ""
			f := &projectSyncForm{open: true, project: a.projects[0], target: other, folder: "~/code/antares", step: syncDone}
			f.result[0].Notes = []string{"A note of what happened on it, long enough to wrap inside the dialog as a real one may, and then some more of it"}
			a.projectSyncing = f
		}},
		{name: "project-links-empty", open: func(a *App) { a.page = ""; a.openLinks(a.projects[0]) }},
		{name: "project-links-copies", open: func(a *App) { auditLinks(a) }},
		{name: "project-links-settings", open: func(a *App) {
			f := auditLinks(a)
			l := a.links.links["l1"]
			f.editing, f.draft, f.fields = "l1", l.ProjectLink, linkFieldsOf(l.ProjectLink)
		}},
		{name: "project-links-push", open: func(a *App) { f := auditLinks(a); f.tab, f.project.IsGit = linksTabPush, true }},
		{name: "project-links-push-elsewhere", open: func(a *App) {
			f := auditLinks(a)
			f.tab, f.project.IsGit = linksTabPush, true
			f.push.Host, f.push.HostName, f.push.Err, f.push.Visibility = "labs", "enowxlabs", "", ""
		}},
		{name: "project-links-no-git", open: func(a *App) { f := auditLinks(a); f.tab, f.project.IsGit = linksTabPush, false }},
		{name: "project-links-history", open: func(a *App) { f := auditLinks(a); f.tab = 2 }},
		{name: "project-conflict-diff", open: func(a *App) {
			f := auditLinks(a)
			l := a.links.links["l1"]
			f.diff = &conflictDiff{link: l.ProjectLink, file: l.Status.Conflicts[0], lines: []diffLine{{' ', "same"}, {'-', "the source's"}, {'+', "the copy's, and a long line that has to wrap inside the box it shows in, whatever the width of the dialog, and more"}}}
		}},
		{name: "project-conflict-diff-err", open: func(a *App) {
			f := auditLinks(a)
			l := a.links.links["l1"]
			f.diff = &conflictDiff{link: l.ProjectLink, file: l.Status.Conflicts[0], err: "Not text: pick a version without comparing"}
		}},
		{name: "project-group-add", open: func(a *App) {
			a.page = ""
			a.groupForm = &groupForm{open: true, project: a.projects[0].Path}
		}},
		{name: "project-group-rename", open: func(a *App) {
			a.page = ""
			a.groupForm = &groupForm{open: true, from: "Clients", name: "Clients"}
		}},
		{name: "project-space-move", open: func(a *App) {
			auditTeamData(a)
			a.page = pageNotes
			n := a.noteList.list[0]
			a.spaceMoving = &spaceMoveForm{open: true, note: &n, title: n.Title, to: "t1", teams: map[string]api.Team{"t1": a.cloudSet.full["t1"]}}
		}},
		{name: "project-space-copy-out-blocked", open: func(a *App) {
			auditTeamData(a)
			a.page = pageNotes
			n := a.noteList.list[0]
			a.spaceMoving = &spaceMoveForm{open: true, note: &n, title: n.Title, from: "t1", to: spacePersonal, copy: true, teams: map[string]api.Team{"t1": a.cloudSet.full["t1"]}, err: "spaces: it holds what looks like a secret, and a reason long enough to wrap"}
		}},
		{name: "project-spaces-gone", open: func(a *App) {
			a.page = ""
			a.goneList = &hostSpacesGone{host: a.host.ID, list: []proto.SpaceGone{{Team: "t", Name: "A team with a name too long for the card it shows in, as it has to run", Shared: true}, {Team: "u", Name: "Another"}}}
		}},
	}
}

// TestLayoutAuditProjects draws them as TestLayoutAudit draws the others.
func TestLayoutAuditProjects(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	auditPagesOn(t, h, projectAuditPages(), func() {
		a := h.app
		a.hostList, a.groupForm, a.spaceMoving, a.goneList = nil, nil, nil, nil
		a.spaceList, a.teamChats, a.teamProjectList, a.teamHostList, a.teamKitList, a.here = nil, nil, nil, nil, nil, nil
		a.links = autoSync{}
		a.daemonInfo.ID = ""
	})
}
