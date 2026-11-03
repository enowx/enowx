package ui

import (
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
)

// The states of Settings' RAG and Cloud, and the project's rules, drawn from
// sample data for the layout audit: reaching them for real takes an embedder
// and a cloud.

func auditRAG(a *App) *ragSettingsState {
	a.page, a.settingsSection = pageSettings, sectionRAG
	a.projects = append(a.projects[:1:1],
		proto.ProjectInfo{Path: "/p/enowx-web", Name: "enowx-web"}, proto.ProjectInfo{Path: "/p/enowx-cloud", Name: "enowx-cloud with a rather long name to run"},
		proto.ProjectInfo{Path: "/p/mygo", Name: "mygo"}, proto.ProjectInfo{Path: "/p/antares", Name: "antares"})
	dirs := make([]string, len(a.projects))
	for i, p := range a.projects {
		dirs[i] = p.Path
	}
	now := time.Now()
	done := proto.RAGStatus{Project: dirs[0], Ready: true, Embedder: "voyage api.voyageai.com", Chunks: 4212, Files: 318, Skipped: 2, Added: 14, Removed: 3, Tokens: 186000, Indexed: now,
		Settings: proto.RAGProject{On: true, Consent: "voyage api.voyageai.com", Context: true, Offer: true, Exclude: []string{"testdata/"}}}
	changed := done
	changed.Settings.Consent = "someone else"
	indexing := done
	indexing.Indexing, indexing.Phase = true, "embedding"
	failed := done
	failed.Error = "the embedder said 401: check its key, and this error runs on long enough to need its lines to wrap inside the card"
	a.ragSet = ragSettingsState{host: a.host.ID, asked: true, loaded: true,
		set: proto.RAGSettings{Embedder: proto.RAGVoyage, Model: "voyage-code-3", HasKey: true},
		dim: "1024", test: "Works: vectors of 1024 dimensions, the store answers.", testOK: true,
		projects: ragProjects{asked: strings.Join(dirs, "\x00"), errs: map[string]string{}, status: map[string]proto.RAGStatus{
			dirs[0]: done, dirs[1]: changed, dirs[2]: indexing, dirs[3]: failed, dirs[4]: {Project: dirs[4], Ready: true},
		}}}
	return &a.ragSet
}

func auditCloud(a *App) *cloudSettings {
	a.page, a.settingsSection = pageSettings, sectionCloud
	now := time.Now()
	me := api.Account{ID: "me", Name: "enowdev", GitHub: "enowdev"}
	rina := api.Account{ID: "rina", Name: "Rina", GitHub: "rina"}
	bayu := api.Account{ID: "bayu", Name: "Bayu with a long name that has to run", GitHub: "bayu"}
	t1 := api.Team{ID: "t1", Name: "enowX", Role: api.RoleOwner, Created: now, Members: []api.Member{{Account: me, Role: api.RoleOwner}, {Account: rina, Role: api.RoleMember}, {Account: bayu, Role: api.RoleViewer}}}
	t2 := api.Team{ID: "t2", Name: "A team whose name is too long for its card and has to run", Role: api.RoleViewer, Created: now, Members: []api.Member{{Account: rina, Role: api.RoleOwner}, {Account: me, Role: api.RoleViewer}}}
	a.cloudSet = &cloudSettings{host: a.host.ID, loaded: true,
		st: proto.CloudStatus{SignedIn: true, Account: "enowdev", AccountID: "me", GitHub: "enowdev", DeviceKind: proto.CloudDeviceApp, DeviceName: a.host.Name, URL: "https://cloud.example.com"},
		devices: []api.Device{
			{ID: "d1", Name: a.host.Name, Kind: api.DeviceApp, Current: true, LastSeen: now},
			{ID: "d2", Name: "enowxlabs, a server with a long name that has to run", Kind: api.DeviceHost, LastSeen: now.Add(-time.Hour)},
			{ID: "d3", Name: "recovery", Recovery: true},
		},
		teams: cloudTeams{list: []api.Team{{ID: "t1", Name: t1.Name, Role: t1.Role}, {ID: "t2", Name: t2.Name, Role: t2.Role}}},
		full:  map[string]api.Team{"t1": t1, "t2": t2},
		latest: []cloudRecord{
			{team: "enowX", entry: api.AuditEntry{ID: 1, At: now, Account: me, Action: "team.created"}},
			{team: "enowX", entry: api.AuditEntry{ID: 2, At: now.Add(-24 * time.Hour), Account: rina, Action: "member.joined", Detail: "as a member, and a detail long enough to run in its row whatever the window's width is, which is what it does"}},
		},
	}
	return a.cloudSet
}

// auditOpenTeam opens the team's sheet on tab.
func auditOpenTeam(a *App, tab string) *cloudSettings {
	cs := auditCloud(a)
	cs.teams.open, cs.teams.team, cs.teams.tab = "t1", cs.full["t1"], tab
	return cs
}

func auditCloudPages() []auditPage {
	signIn := func(a *App, st proto.CloudStatus) {
		cs := auditCloud(a)
		cs.st = st
		cs.opened = "WDJB-MJHT" // no browser opens for it
		a.cloudSigningIn = true
	}
	return []auditPage{
		{name: "settings-rag-states", open: func(a *App) { auditRAG(a) }, fill: true},
		{name: "settings-rag-none", open: func(a *App) { auditRAG(a).set.Embedder = "" }, fill: true},
		{name: "settings-rag-postgres", open: func(a *App) {
			st := auditRAG(a)
			st.set.Embedder, st.set.BaseURL, st.set.Store, st.set.HasPostgres = proto.RAGOpenAI, "https://api.openai.com/v1", proto.RAGPostgres, true
			st.test, st.testOK = "the store refused the connection: password authentication failed, and more of it so that it wraps inside its notice", false
		}, fill: true},
		{name: "dialog-rag-details", open: func(a *App) { st := auditRAG(a); st.projects.details = a.projects[0].Path }},
		{name: "dialog-rag-consent", open: func(a *App) {
			st := auditRAG(a)
			st.projects.planFor = a.projects[4].Path
			st.projects.plan = &proto.RAGPlan{Embedder: "Voyage AI", Host: "api.voyageai.com", Files: 212, Tokens: 340000, Left: []string{"Secrets (.env, keys, files that look like credentials)", "what .gitignore leaves out"}}
		}},
		{name: "dialog-rag-offer", open: func(a *App) {
			a.page = ""
			a.ragOffers = []*proto.RAGRememberParams{{RAGRef: proto.RAGRef{Project: a.projects[0].Path}, Text: "We keep the index in SQLite unless a project passes a hundred thousand parts."}}
		}},
		{name: "settings-cloud-teams", open: func(a *App) { auditCloud(a) }, fill: true},
		{name: "settings-cloud-no-teams", open: func(a *App) {
			cs := auditCloud(a)
			cs.teams.list, cs.full, cs.latest = nil, nil, nil
			cs.err, cs.busy = "The cloud can't be reached: and this line goes on long enough to have to wrap inside its notice, for it to be seen doing that.", "Making a team…"
		}, fill: true},
		{name: "settings-cloud-devices", open: func(a *App) { auditCloud(a).tab = 1 }, fill: true},
		{name: "settings-cloud-out", open: func(a *App) { auditCloud(a).st = proto.CloudStatus{} }, fill: true},
		{name: "settings-cloud-pending", open: func(a *App) {
			auditCloud(a).st = proto.CloudStatus{Pending: &proto.CloudPending{UserCode: "WDJB-MJHT", Expires: time.Now().Add(time.Hour)}}
		}, fill: true},
		{name: "settings-cloud-asking", open: func(a *App) { auditCloud(a).loaded = false }, fill: true},
		{name: "dialog-cloud-team", open: func(a *App) { auditOpenTeam(a, tabMembers) }},
		{name: "dialog-cloud-team-invite", open: func(a *App) {
			auditOpenTeam(a, tabInvite).teams.invite = api.Invite{Token: "sample", URL: "https://cloud.example.com/invite#enxi_sample_not_real_and_long_enough_to_have_to_run_inside_its_box_whatever_the_width", Role: api.RoleMember, Expires: time.Now()}
		}},
		{name: "dialog-cloud-team-options", open: func(a *App) { auditOpenTeam(a, tabOptions) }},
		{name: "dialog-cloud-new-team", open: func(a *App) { auditCloud(a).teams.making = true }},
		{name: "dialog-cloud-join", open: func(a *App) { auditCloud(a).teams.joining = true }},
		{name: "dialog-cloud-recovery", open: func(a *App) { c := auditCloud(a); c.tab, c.recovery.shown = 1, "SAMPLE-RECOVERY-KEY-NOT-REAL" }},
		{name: "dialog-cloud-recovery-use", open: func(a *App) { c := auditCloud(a); c.tab, c.recovery.using = 1, true }},
		{name: "dialog-cloud-signin-start", open: func(a *App) { signIn(a, proto.CloudStatus{}) }},
		{name: "dialog-cloud-signin-waiting", open: func(a *App) {
			signIn(a, proto.CloudStatus{Pending: &proto.CloudPending{UserCode: "WDJB-MJHT", VerifyURLComplete: "https://cloud.example.com/device", Expires: time.Now().Add(time.Hour)}})
		}},
		{name: "dialog-cloud-signin-done", open: func(a *App) {
			signIn(a, proto.CloudStatus{SignedIn: true, Account: "enowdev", GitHub: "enowdev", DeviceKind: proto.CloudDeviceApp})
		}},
		{name: "dialog-rules", open: func(a *App) {
			a.page = ""
			a.rulesEditing = &rulesForm{open: true, project: a.projects[0], loaded: true, text: "- Run the tests first", rules: proto.ProjectRules{CanEdit: true},
				draft: rulesDraft{asked: true, by: "claude", conflicts: []string{"It says to skip the tests, against the first rule."}, versions: []proto.RulesVersion{{Time: time.Now(), Rules: "- An earlier rule"}}}}
		}},
		{name: "dialog-rules-team", open: func(a *App) {
			a.page = ""
			a.rulesEditing = &rulesForm{open: true, project: a.projects[0], loaded: true, text: "- The team's rule", rules: proto.ProjectRules{Team: "t1"}, draft: rulesDraft{asked: true}}
		}},
	}
}
