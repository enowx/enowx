package ui

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
)

// A team's page, side panel tools and dialogs drawn from sample data, for the
// layout audit: a real team takes a cloud.

// auditTeamData fills in what a host in two teams would have read.
func auditTeamData(a *App) *cloudSettings {
	cs := auditCloud(a)
	host := a.host.ID
	cs.st = proto.CloudStatus{SignedIn: true, Account: "enowdev", GitHub: "enowdev", AccountID: "me", DeviceKind: proto.CloudDeviceApp}
	a.spaceList = &hostSpaces{host: host, list: []proto.Space{{Team: "t1", Name: "enowX", Role: api.RoleOwner}, {Team: "t2", Name: cs.full["t2"].Name, Role: api.RoleViewer}}}
	now := time.Now()
	dir := a.projects[0].Path
	a.teamProjectList = &hostTeamProjects{host: host, list: []proto.TeamProject{
		{ID: "p1", Team: "t1", Name: "antares", Repo: "https://github.com/enowx/antares", Ref: "git:github.com/enowx/antares", AddedBy: "enowdev", Here: []proto.TeamProjectCopy{{Path: dir, Project: true}}},
		{ID: "p2", Team: "t1", Name: "enowx-cloud with a rather long name that has to run in its card", Repo: "https://github.com/enowx/enowx-cloud", Ref: "git:github.com/enowx/enowx-cloud-and-a-long-way-after-that-to-run-inside-its-row"},
	}}
	a.teamHostList = &hostTeamHosts{host: host,
		share: proto.HostShare{Addr: "100.74.12.8:7422", Teams: []proto.HostShareTeam{{Team: "t1", Projects: []string{dir}, AllowSessions: new(bool)}}},
		hosts: []proto.TeamHost{{Team: "t1", Device: "d1", Name: "studio-mac", Owner: "Rina", Addr: "100.1.1.1:7422", Projects: []proto.TeamHostProject{{Name: "enowx", Path: "/p/enowx"}, {Name: "enowx-web", Path: "/p/enowx-web"}}}}}
	stages, _ := json.Marshal([]proto.PipelineStage{{Name: "Plan"}, {Name: "Build"}, {Name: "Review"}})
	auto, _ := json.Marshal(proto.Automation{Name: "Nightly dependency check", Agent: "claude", Schedule: "daily 02:00", Project: "git:github.com/enowx/antares"})
	tmpl, _ := json.Marshal(proto.KitTemplateData{Repo: "https://github.com/enowx/template"})
	a.teamKitList = &hostTeamKit{host: host, list: []proto.TeamKitItem{
		{Team: "t1", Kind: proto.EnvSkill, Name: "concise-context", About: "Keeps replies short, and a description long enough that it can't all fit in the card it shows in at any size of the window"},
		{Team: "t1", Kind: proto.EnvCommand, Name: "release"},
		{Team: "t1", Kind: proto.KitPipeline, Name: "Plan, build, review", Data: stages},
		{Team: "t1", Kind: proto.KitAutomation, Name: "nightly-dependency-check", Data: auto},
		{Team: "t1", Kind: proto.KitTemplate, Name: "Service", Data: tmpl},
	}, items: []proto.EnvItem{
		{ID: "claude/skill/handoff", Agent: "claude", Kind: proto.EnvSkill, Name: "handoff"},
		{ID: "claude/skill/concise-context", Agent: "claude", Kind: proto.EnvSkill, Name: "concise-context"},
		{ID: "claude/command/deploy", Agent: "claude", Kind: proto.EnvCommand, Name: "deploy", Secrets: []string{"TOKEN"}},
	}}
	a.hereOf().byTeam["t1"] = api.Presence{Online: []string{"me", "rina"}, Typing: []string{"rina"}}
	hc := a.chatsOf()
	hc.members["t1"], hc.membersAt["t1"] = cs.full["t1"].Members, now.Add(time.Hour)
	hc.members["t2"], hc.membersAt["t2"] = cs.full["t2"].Members, now.Add(time.Hour)
	hc.pulled = now.Add(time.Hour)
	hc.readGen["t1"], hc.readGen["t2"] = a.gen, a.gen
	hc.readTried["t1"] = now
	hc.byTeam["t1"] = []proto.ChatMessage{
		{ID: "1", Team: "t1", Account: "rina", Name: "Rina", GitHub: "rina", At: now.Add(-25 * time.Hour), Text: "Can someone look at the footer test on enowxlabs?"},
		{ID: "2", Team: "t1", Account: "me", Name: "enowdev", At: now.Add(-3 * time.Hour), Text: "On it, Codex has it open.", ReplyTo: "1"},
		{ID: "3", Team: "t1", Account: "bayu", Name: "Bayu", GitHub: "bayu", At: now.Add(-time.Hour), Text: "The 0.5.0 page is up, @enowdev. It says so at canvas://abc123/design-1, and more words so that this message has to wrap inside its width, wherever it shows.",
			Mentions: []proto.ChatMention{{Kind: proto.MentionUser, ID: "me", Title: "enowdev"}, {Kind: proto.MentionNote, ID: "n", Title: "Release plan"}}, Reactions: []proto.ChatReaction{{Emoji: "👍", Accounts: []string{"me", "rina"}}}},
		{ID: "4", Team: "t1", Account: "bayu", Name: "Bayu", GitHub: "bayu", At: now.Add(-time.Hour + time.Minute), Text: "and a second one, edited", Edited: &now},
	}
	cs.teams.audit = &teamAudit{team: "t1", loaded: true, entries: []api.AuditEntry{
		{ID: 1, At: now, Account: api.Account{ID: "me", Name: "enowdev"}, Action: "team.created"},
		{ID: 2, At: now.Add(-24 * time.Hour), Account: api.Account{ID: "rina", Name: "Rina"}, Action: "member.joined", Detail: "as a member, and a detail long enough to run in its row whatever the window's width is, which is what it does"},
		{ID: 3, At: now.Add(-50 * time.Hour), Action: "key.shared"},
	}}
	return cs
}

// auditTeamPage shows team on the Team page, on tab.
func auditTeamPage(a *App, team, tab string) *cloudSettings {
	cs := auditTeamData(a)
	cs.teams.open, cs.teams.pageTeam, cs.teams.team, cs.teams.pageTab = team, team, cs.full[team], tab
	a.page = pageTeam
	return cs
}

func teamAuditPages() []auditPage {
	pages := []auditPage{}
	for _, tab := range teamPageTabs {
		pages = append(pages, auditPage{name: "team-page-" + tab, fill: true, open: func(a *App) { auditTeamPage(a, "t1", tab) }})
		pages = append(pages, auditPage{name: "team-viewer-" + tab, fill: true, open: func(a *App) { auditTeamPage(a, "t2", tab) }})
	}
	panel := func(tool string) func(a *App) {
		return func(a *App) {
			auditTeamData(a)
			a.page, a.state.Panel.Team = "", "t1"
			a.selectSession(a.sessions[0])
			a.openPanel(tool)
		}
	}
	dialog := func(tab string) func(a *App) { return func(a *App) { auditOpenTeam2(a, tab) } }
	return append(pages,
		auditPage{name: "team-none", fill: true, open: func(a *App) {
			cs := auditTeamData(a)
			cs.teams.list, a.page = nil, pageTeam
		}},
		auditPage{name: "team-signed-out", fill: true, open: func(a *App) {
			cs := auditTeamData(a)
			cs.st, a.page = proto.CloudStatus{}, pageTeam
		}},
		auditPage{name: "team-new", open: func(a *App) { auditTeamPage(a, "t1", tabChat).teams.making = true }},
		auditPage{name: "team-join", open: func(a *App) { auditTeamPage(a, "t1", tabChat).teams.joining = true }},
		auditPage{name: "team-invite", open: func(a *App) { auditTeamPage(a, "t1", tabMembers).teams.inviting = true }},
		auditPage{name: "team-share-kit", open: func(a *App) { auditTeamPage(a, "t1", tabKit).teams.sharing = true }},
		auditPage{name: "team-delete", open: func(a *App) { auditTeamPage(a, "t1", tabOptions).teams.deleting = true }},
		auditPage{name: "team-panel-chat", open: panel(panelChat)},
		auditPage{name: "team-panel-projects", open: panel(panelTeamProjects)},
		auditPage{name: "team-panel-members", open: panel(panelTeamMembers)},
		auditPage{name: "team-dialog-projects", open: dialog(tabProjects)},
		auditPage{name: "team-dialog-hosts", open: dialog(tabHosts)},
		auditPage{name: "team-dialog-kit", open: dialog(tabKit)},
		auditPage{name: "team-dialog-activity", open: dialog(tabActivity)},
		auditPage{name: "team-dialog-options", open: dialog(tabOptions)},
	)
}

// auditOpenTeam2 opens Settings' dialog of the first team, on tab, with the
// data of its tabs.
func auditOpenTeam2(a *App, tab string) {
	cs := auditTeamData(a)
	a.page = pageSettings
	cs.teams.open, cs.teams.team, cs.teams.tab = "t1", cs.full["t1"], tab
}

// TestLayoutAuditTeam draws them as TestLayoutAudit draws the others.
func TestLayoutAuditTeam(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	auditPagesOn(t, h, teamAuditPages(), func() {
		h.app.spaceList, h.app.teamChats, h.app.teamProjectList, h.app.teamHostList, h.app.teamKitList, h.app.here = nil, nil, nil, nil, nil, nil
	})
}

// More reaches the Team page; its tabs show the team's chat, cards and options,
// the side list changes team, and leaving it doesn't open Settings' dialog.
func TestTeamPage(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	a := h.app
	cs := auditTeamData(a)
	log := cs.teams.audit // opening the team reads it again, from a cloud that isn't here
	a.page = ""
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1280, 800)
	h.tt = tt
	step := func(what string, done func() bool) {
		t.Helper()
		h.settle(what, done)
	}
	click := func(label string) {
		t.Helper()
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %s: %v; shows %q", label, err, tt.Texts())
		}
		tt.Frame()
	}
	click("More")
	click("Team")
	step("the chat of the first team", func() bool {
		return a.page == pageTeam && tt.HasText("Teams") && tt.HasText("Can someone look at the footer test on enowxlabs?")
	})
	if a.teamPageChat() != "t1" || !a.chatShows("t1") {
		t.Fatalf("its chat should count as shown: %q", a.teamPageChat())
	}
	cs.teams.audit = log
	click(tabProjects)
	step("the projects", func() bool { return tt.HasText("Add a project") && tt.HasText("Shared hosts") })
	click(tabMembers)
	step("the members", func() bool { return tt.HasText("Latest in the team") && tt.HasText("Rina") })
	click("Invite")
	step("the invite dialog", func() bool { return tt.HasText("Invite to enowX") && tt.HasText("Make a link") })
	cs.teams.inviting = false
	click(tabHosts)
	step("the hosts", func() bool { return tt.HasText("studio-mac") && tt.HasText("Members start sessions here") })
	click(tabKit)
	step("the kit", func() bool { return tt.HasText("Plan, build, review") && tt.HasText("Add a template") })
	click("Share one of yours")
	step("what to share", func() bool { return tt.HasText("handoff") && tt.HasText("holds secrets") })
	cs.teams.sharing = false
	click(tabActivity)
	step("the log", func() bool { return tt.HasText("enowdev made the team") })
	click(tabOptions)
	step("the options", func() bool { return tt.HasText("Copying out") && tt.HasText("Delete the team…") })
	click("Delete the team…")
	step("the delete dialog", func() bool { return tt.HasText("Type enowX to confirm") })
	cs.teams.deleting = false
	step("the delete dialog gone", func() bool { return !tt.HasText("Type enowX to confirm") })
	tt.Advance(time.Second) // the frame's still on its way beside the list

	// A viewer's team: no log, no rules.
	cs.full["t2"] = api.Team{ID: "t2", Name: "Antares Labs", Role: api.RoleViewer, Members: cs.full["t2"].Members}
	cs.teams.list[1].Name = "Antares Labs"
	click("Antares Labs")
	step("the other team", func() bool { return cs.teams.open == "t2" && tt.HasText("Leaving") })
	if slices.Contains(teamPageTabsFor(api.RoleViewer), tabActivity) || tt.HasText("Rules") {
		t.Fatalf("a viewer's page offers an admin's: %q", tt.Texts())
	}

	a.page, a.settingsSection = pageSettings, sectionCloud
	step("Settings' teams, no dialog open", func() bool { return tt.HasText("Latest in your teams") && cs.teams.open == "" && !tt.HasText("Done") })
}
