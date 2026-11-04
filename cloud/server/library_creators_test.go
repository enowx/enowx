package server

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
)

func creatorPath(pattern, handle string) string {
	return strings.NewReplacer("{handle}", handle).Replace(pattern)
}

func savePath(id string) string { return itemPath(api.PathLibrarySave, id) }

func collectionItemPath(id, item string) string {
	return strings.NewReplacer("{id}", id, "{item}", item).Replace(api.PathLibraryCollectionItem)
}

// live publishes a design and approves it, so it shows.
func (e *libEnv) liveAs(m signedInMember, title string) string {
	e.t.Helper()
	sub := e.publish(m, publishParams(title), `<p>x</p>`)
	if sub.Status == api.LibraryLive {
		return sub.ID
	}
	if code, msg := e.decide(e.webSession(e.admin.acc), sub.ID, 1, api.DecisionApprove, "ok"); code != http.StatusNoContent {
		e.t.Fatalf("approve: %d %s", code, msg)
	}
	return sub.ID
}

func (e *libEnv) setProfile(token string, p api.LibraryProfileParams) (int, string) {
	e.t.Helper()
	return send(e.t, "PUT", e.url+api.PathLibraryMe, token, p, nil)
}

func (e *libEnv) mustProfile(token string, p api.LibraryProfileParams) {
	e.t.Helper()
	if code, msg := e.setProfile(token, p); code != http.StatusOK {
		e.t.Fatalf("set profile: %d %s", code, msg)
	}
}

func (e *libEnv) profile(handle, token string) (int, api.LibraryProfile) {
	e.t.Helper()
	var out api.LibraryProfile
	code, _ := send(e.t, "GET", e.url+creatorPath(api.PathLibraryCreator, handle), token, nil, &out)
	return code, out
}

func listIDs(items []api.LibraryItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// A handle, a link and a bio keep to the rules; what's wrong says so.
func TestLibraryProfileRules(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	bob := e.signIn(2, "bob", "")
	e.mustProfile(bob.token, api.LibraryProfileParams{Handle: "bobby"})
	next := int64(100)
	for name, p := range map[string]api.LibraryProfileParams{
		"short handle":       {Handle: "ab"},
		"long handle":        {Handle: strings.Repeat("a", 31)},
		"shape":              {Handle: "a--b"},
		"edge hyphen":        {Handle: "-abc"},
		"symbols":            {Handle: "a_b.c"},
		"kept handle":        {Handle: "admin"},
		"deleted handle":     {Handle: "deleted-x"},
		"taken handle":       {Handle: "bobby"},
		"their github":       {Handle: "bob"},
		"javascript link":    {Links: []string{"javascript:alert(1)"}},
		"data link":          {Links: []string{"data:text/html,x"}},
		"mailto link":        {Links: []string{"mailto:ada@example.com"}},
		"no scheme":          {Links: []string{"example.com"}},
		"name in link":       {Links: []string{"https://user:pw@example.com"}},
		"email in link":      {Links: []string{"https://example.com/ada@example.com"}},
		"too many links":     {Links: []string{"https://a.example", "https://b.example", "https://c.example", "https://d.example", "https://e.example", "https://f.example"}},
		"email in bio":       {Bio: "Reach me at ada@example.com"},
		"mailto contact":     {OpenForWork: true, ContactURL: "mailto:ada@example.com"},
		"javascript contact": {ContactURL: "javascript:alert(1)"},
		"open, no contact":   {OpenForWork: true},
		"unknown work":       {WorkKinds: []string{"slavery"}},
		"bad zone":           {TimeZone: "Jakarta"},
		"zone too far":       {TimeZone: "UTC+15"},
		"too many skills":    {Skills: strings.Split("a b c d e f g h i j k l m", " ")},
	} {
		next++
		who := e.signIn(next, "rules-"+strconv.FormatInt(next, 10), "")
		if code, msg := e.setProfile(who.token, p); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, msg)
		}
	}
	// Nothing of those was kept, and text comes back clean.
	e.mustProfile(ada.token, api.LibraryProfileParams{Handle: "Ada-Dev", Bio: " Makes\u202e cards ", Links: []string{"https://a.example/x", "https://a.example/x"}, TimeZone: "gmt+7", Skills: []string{"Landing Pages", "landing pages", ""}})
	code, me := e.profile("ada-dev", ada.token)
	if code != http.StatusOK || me.Bio != "Makes cards" || len(me.Links) != 1 || me.Settings == nil || me.Settings.TimeZone != "UTC+7" || !slices.Equal(me.Settings.Skills, []string{"landing pages"}) {
		t.Fatalf("%d %+v %+v", code, me, me.Settings)
	}
	if code, _ := e.profile("ada", ""); code != http.StatusNotFound {
		t.Fatalf("the old handle: %d", code)
	}
	if code, msg := e.setProfile(ada.token, api.LibraryProfileParams{Handle: "ada-again"}); code != http.StatusBadRequest || !strings.Contains(msg, "30 days") {
		t.Fatalf("a second change soon after: %d %s", code, msg)
	}
	if code, _ := send(t, "PUT", e.url+api.PathLibraryMe, "", api.LibraryProfileParams{}, nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	for in, want := range map[string]string{"UTC+5:30": "UTC+5:30", "-3:30": "UTC-3:30", "7": "UTC+7", "utc": "", "UTC+0": "UTC", "UTC+14": "UTC+14"} {
		code, msg := e.setProfile(ada.token, api.LibraryProfileParams{TimeZone: in})
		if in == "utc" {
			if code != http.StatusBadRequest {
				t.Errorf("%q: %d", in, code)
			}
			continue
		}
		if code != http.StatusOK {
			t.Errorf("%q: %d %s", in, code, msg)
			continue
		}
		_, got := e.profile("ada-dev", ada.token)
		if got.Settings.TimeZone != want {
			t.Errorf("%q: %q, want %q", in, got.Settings.TimeZone, want)
		}
	}
}

// A profile shows what its creator chose, and from a session as from a device.
func TestLibraryProfileShowsWhatItChose(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.signIn(1, "ada", "")
	id := e.liveAs(ada, "Mine")
	web := e.webSession(ada.acc)

	code, anon := e.profile("ada", "")
	if code != http.StatusOK || anon.GitHub != "" || anon.Settings != nil || anon.Viewer != nil || anon.OpenForWork != nil {
		t.Fatalf("before choosing: %d %+v", code, anon)
	}
	if anon.Stats == nil || anon.Stats.Designs != 1 {
		t.Fatalf("stats: %+v", anon.Stats)
	}
	e.mustProfile(web, api.LibraryProfileParams{ShowGitHub: true, ShowStats: false, Bio: "Hi",
		OpenForWork: true, ContactURL: "https://ada.example/hire", WorkKinds: []string{"freelance"}, Skills: []string{"motion"}, TimeZone: "UTC+7"})
	_, anon = e.profile("ada", "")
	if anon.GitHub != "ada" || anon.Stats != nil || anon.OpenForWork == nil || anon.OpenForWork.ContactURL != "https://ada.example/hire" {
		t.Fatalf("after choosing: %+v", anon)
	}
	_, own := e.profile("ada", ada.token)
	if own.Viewer == nil || !own.Viewer.Own || own.Settings == nil || !own.Settings.ShowGitHub {
		t.Fatalf("own view: %+v", own)
	}
	bob := e.signIn(2, "bob", "")
	_, other := e.profile("ada", bob.token)
	if other.Settings != nil || other.Viewer == nil || other.Viewer.Own {
		t.Fatalf("someone else's view: %+v", other)
	}
	// Off, the contact is gone from what anyone gets; what was typed waits for its owner.
	e.mustProfile(ada.token, api.LibraryProfileParams{ShowGitHub: false, ContactURL: "https://ada.example/hire", WorkKinds: []string{"freelance"}})
	raw, _, _ := doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "ada"), "")
	if strings.Contains(raw, "ada.example") || strings.Contains(raw, `"github"`) {
		t.Fatalf("off, it still shows: %s", raw)
	}
	_, own = e.profile("ada", web)
	if own.Settings.ContactURL != "https://ada.example/hire" || own.Settings.OpenForWork {
		t.Fatalf("what was typed: %+v", own.Settings)
	}
	// Works, each way of sorting; a report on the profile reaches moderators.
	second := e.liveAs(ada, "Second")
	if _, err := e.db.SaveLibraryItem(context.Background(), bob.acc.ID, id); err != nil {
		t.Fatal(err)
	}
	for query, first := range map[string]string{"": second, "?sort=popular": id} {
		var works api.LibraryItems
		send(t, "GET", e.url+creatorPath(api.PathLibraryCreatorItems, "ada")+query, "", nil, &works)
		if len(works.Items) != 2 || works.Items[0].ID != first {
			t.Errorf("works %q: %v", query, listIDs(works.Items))
		}
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryReports, e.webSession(bob.acc), api.LibraryReportParams{AccountID: ada.acc.ID, Reason: "impersonation", Detail: "not them"}, nil); code != http.StatusNoContent {
		t.Fatalf("report: %d %s", code, msg)
	}
	var q api.LibraryQueue
	send(t, "GET", e.url+api.PathLibraryQueue, e.webSession(e.admin.acc), nil, &q)
	if len(q.Reports) != 1 || q.Reports[0].AccountID != ada.acc.ID {
		t.Fatalf("reports: %+v", q.Reports)
	}
	// Blocked: no profile to anyone but moderators, and it can't change one.
	boss := e.webSession(e.admin.acc)
	if code, msg := send(t, "POST", e.url+itemPath(api.PathLibraryBlock, ada.acc.ID), boss, api.LibraryReason{Reason: "spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, msg)
	}
	if code, _ := e.profile("ada", ""); code != http.StatusNotFound {
		t.Fatalf("blocked, anyone: %d", code)
	}
	if code, _ := e.profile("ada", boss); code != http.StatusOK {
		t.Fatalf("blocked, a moderator: %d", code)
	}
	if code, _ := e.setProfile(ada.token, api.LibraryProfileParams{Bio: "still here"}); code != http.StatusForbidden {
		t.Fatalf("blocked, editing: %d", code)
	}
}

// The list needs a design that passed and a way to reach someone; its filters work;
// off or hidden, they leave it.
func TestLibraryOpenForWorkList(t *testing.T) {
	e := newLibEnv(t, Config{})
	list := func(query string) api.LibraryOpenCreators {
		t.Helper()
		var out api.LibraryOpenCreators
		if code, msg := send(t, "GET", e.url+api.PathLibraryCreators+"?open=1"+query, "", nil, &out); code != http.StatusOK {
			t.Fatalf("%q: %d %s", query, code, msg)
		}
		return out
	}
	handles := func(query string) []string {
		t.Helper()
		out := []string{}
		for _, c := range list(query).Creators {
			out = append(out, c.Handle)
		}
		slices.Sort(out)
		return out
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryCreators, "", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("a list of everyone: %d", code)
	}
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	e.mustProfile(ada.token, api.LibraryProfileParams{OpenForWork: true, ContactURL: "https://ada.example", WorkKinds: []string{"freelance", "part-time"}, Skills: []string{"Motion", "R&D"}, TimeZone: "UTC+7"})
	if got := handles(""); len(got) != 0 {
		t.Fatalf("listed with nothing published: %v", got)
	}
	e.liveAs(ada, "A1")
	e.liveAs(bob, "B1")
	e.mustProfile(bob.token, api.LibraryProfileParams{OpenForWork: true, ContactURL: "https://bob.example", WorkKinds: []string{"full-time"}, Skills: []string{"Dashboards", "motion"}, TimeZone: "UTC-5"})
	for query, want := range map[string][]string{
		"":                          {"ada", "bob"},
		"&kind=freelance":           {"ada"},
		"&kind=full-time":           {"bob"},
		"&skill=motion":             {"ada", "bob"},
		"&skill=Dashboards":         {"bob"},
		"&skill=r%26d":              {"ada"},
		"&skill=mo":                 {},
		"&skill=%25":                {},
		"&zone=east":                {"ada"},
		"&zone=west":                {"bob"},
		"&zone=middle":              {},
		"&zone=east&kind=full-time": {},
	} {
		if got := handles(query); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", query, got, want)
		}
	}
	for _, query := range []string{"&kind=slavery", "&zone=mars"} {
		if code, _ := send(t, "GET", e.url+api.PathLibraryCreators+"?open=1"+query, "", nil, nil); code != http.StatusBadRequest {
			t.Errorf("%q: %d", query, code)
		}
	}
	page := list("&limit=1")
	if page.Next == "" || len(page.Creators) != 1 || len(page.Creators[0].Works) != 1 || page.Skills[0] != "motion" {
		t.Fatalf("a page: %+v", page)
	}
	e.mustProfile(bob.token, api.LibraryProfileParams{OpenForWork: false, ContactURL: "https://bob.example"})
	if got := handles(""); !slices.Equal(got, []string{"ada"}) {
		t.Fatalf("off: %v", got)
	}
	e.mustProfile(ada.token, api.LibraryProfileParams{Hidden: true, OpenForWork: true, ContactURL: "https://ada.example"})
	if got := handles(""); len(got) != 0 {
		t.Fatalf("hidden: %v", got)
	}
	// Turning off and hiding go through past the limit: any time means any time.
	carol := e.signIn(3, "carol", "")
	e.mustProfile(carol.token, api.LibraryProfileParams{OpenForWork: true, ContactURL: "https://carol.example"})
	refused := false
	for range 40 {
		if code, _ := e.setProfile(carol.token, api.LibraryProfileParams{OpenForWork: true, ContactURL: "https://carol.example", Bio: "again"}); code == http.StatusTooManyRequests {
			refused = true
			break
		}
	}
	if !refused {
		t.Fatal("no limit on changing a profile")
	}
	if code, msg := e.setProfile(carol.token, api.LibraryProfileParams{ContactURL: "https://carol.example"}); code != http.StatusOK {
		t.Fatalf("turning off, past the limit: %d %s", code, msg)
	}
}

// Hidden, a profile answers as none does, its designs leave every list, and each still
// answers at its address, saying it's unlisted, so copies keep their credit.
func TestLibraryHiddenProfile(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	boss := e.webSession(e.admin.acc)
	id := e.liveAs(ada, "Hide me")
	var pub api.LibraryCollection
	send(t, "POST", e.url+api.PathLibraryCollections, ada.token, api.LibraryCollectionParams{Title: "Ada's", Public: true}, &pub)
	var bobs api.LibraryCollection
	send(t, "POST", e.url+api.PathLibraryCollections, bob.token, api.LibraryCollectionParams{Title: "Bob's", Public: true}, &bobs)
	if code, msg := send(t, "PUT", e.url+collectionItemPath(bobs.ID, id), bob.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("collect: %d %s", code, msg)
	}
	send(t, "PUT", e.url+creatorPath(api.PathLibraryFollow, "ada"), bob.token, nil, nil)
	send(t, "PUT", e.url+savePath(id), bob.token, nil, nil)

	var before api.LibraryFollowing
	send(t, "GET", e.url+api.PathLibraryFollowing, bob.token, nil, &before)
	if len(before.Creators) != 1 {
		t.Fatalf("following: %+v", before)
	}

	e.mustProfile(ada.token, api.LibraryProfileParams{Hidden: true})

	_, _, _ = doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "nobody"), "")
	unknown, unknownCode, _ := doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "nobody"), "")
	hidden, hiddenCode, _ := doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "ada"), bob.token)
	if hiddenCode != http.StatusNotFound || unknownCode != hiddenCode || hidden != unknown {
		t.Fatalf("hidden answers apart from none: %d %q, %d %q", hiddenCode, hidden, unknownCode, unknown)
	}
	for _, path := range []string{api.PathLibraryCreatorItems, api.PathLibraryCreatorCollections} {
		if code, _ := send(t, "GET", e.url+creatorPath(path, "ada"), "", nil, nil); code != http.StatusNotFound {
			t.Errorf("%s: %d", path, code)
		}
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryCollection, pub.ID), bob.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("their public collection: %d", code)
	}
	if code, p := e.profile("ada", ada.token); code != http.StatusOK || !p.Hidden {
		t.Fatalf("to themselves: %d %+v", code, p)
	}
	if code, p := e.profile("ada", boss); code != http.StatusOK || !p.Hidden || p.Settings != nil {
		t.Fatalf("to a moderator: %d %+v", code, p)
	}
	if code, _ := send(t, "PUT", e.url+creatorPath(api.PathLibraryFollow, "ada"), boss, nil, nil); code != http.StatusNotFound {
		t.Fatalf("following one hidden, as a moderator: %d", code)
	}
	var browse api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryItems, "", nil, &browse)
	if len(browse.Items) != 0 {
		t.Fatalf("browse: %v", listIDs(browse.Items))
	}
	var feed api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryFeed, bob.token, nil, &feed)
	if len(feed.Items) != 0 {
		t.Fatalf("feed: %v", listIDs(feed.Items))
	}
	var following api.LibraryFollowing
	send(t, "GET", e.url+api.PathLibraryFollowing, bob.token, nil, &following)
	if len(following.Creators) != 0 {
		t.Fatalf("following: %+v", following)
	}
	// In someone else's public collection it's gone for readers, there for its owner.
	var page api.LibraryCollectionPage
	send(t, "GET", e.url+itemPath(api.PathLibraryCollection, bobs.ID), "", nil, &page)
	if len(page.Items) != 0 || page.Collection.Items != 0 {
		t.Fatalf("a reader's view of a collection: %+v", page)
	}
	send(t, "GET", e.url+itemPath(api.PathLibraryCollection, bobs.ID), bob.token, nil, &page)
	if len(page.Items) != 1 || !page.Own {
		t.Fatalf("its owner's view: %+v", page)
	}
	var liked api.LibraryItems
	send(t, "GET", e.url+api.PathLibrarySaves, bob.token, nil, &liked)
	if len(liked.Items) != 1 {
		t.Fatalf("what bob liked: %+v", liked)
	}
	var it api.LibraryItem
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, &it); code != http.StatusOK || !it.Unlisted || it.Creator.Handle != "ada" {
		t.Fatalf("at its address: %d %+v", code, it)
	}
	var prompt api.LibraryPrompt
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryPrompt, id), "", nil, &prompt); code != http.StatusOK || !strings.Contains(prompt.Prompt, "by ada") {
		t.Fatalf("its credit: %d %+v", code, prompt)
	}
	// Back, it's all as it was.
	e.mustProfile(ada.token, api.LibraryProfileParams{})
	send(t, "GET", e.url+api.PathLibraryItems, "", nil, &browse)
	send(t, "GET", e.url+api.PathLibraryFeed, bob.token, nil, &feed)
	var again api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, &again)
	if len(browse.Items) != 1 || len(feed.Items) != 1 || again.Unlisted {
		t.Fatalf("shown again: %v %v %+v", listIDs(browse.Items), listIDs(feed.Items), again.Unlisted)
	}
}

// Likes count once, are the asker's alone to read, and take either kind of token.
func TestLibraryLikes(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada, bob, carol := e.signIn(1, "ada", ""), e.signIn(2, "bob", ""), e.signIn(3, "carol", "")
	id := e.liveAs(ada, "Liked")
	web := e.webSession(bob.acc)
	if code, _ := send(t, "PUT", e.url+savePath(id), "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	if code, _ := send(t, "PUT", e.url+savePath(id), ada.token, nil, nil); code != http.StatusBadRequest {
		t.Fatalf("liking one's own: %d", code)
	}
	var st api.LibrarySaved
	for _, token := range []string{bob.token, web, bob.token} {
		if code, msg := send(t, "PUT", e.url+savePath(id), token, nil, &st); code != http.StatusOK || !st.Saved || st.Saves != 1 {
			t.Fatalf("like: %d %s %+v", code, msg, st)
		}
	}
	send(t, "PUT", e.url+savePath(id), carol.token, nil, &st)
	if st.Saves != 2 {
		t.Fatalf("two likes: %+v", st)
	}
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, &it)
	if it.Saves != 2 {
		t.Fatalf("the item's count: %d", it.Saves)
	}
	send(t, "GET", e.url+savePath(id), ada.token, nil, &st)
	if st.Saved || st.Saves != 2 {
		t.Fatalf("to someone who hasn't: %+v", st)
	}
	// What someone likes is theirs alone: no route names another, and nothing public says.
	var mine, others api.LibraryItems
	send(t, "GET", e.url+api.PathLibrarySaves, bob.token, nil, &mine)
	send(t, "GET", e.url+api.PathLibrarySaves, ada.token, nil, &others)
	if len(mine.Items) != 1 || len(others.Items) != 0 {
		t.Fatalf("lists: %v %v", listIDs(mine.Items), listIDs(others.Items))
	}
	if code, _ := send(t, "GET", e.url+api.PathLibrarySaves, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the list signed out: %d", code)
	}
	raw, _, _ := doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "bob"), "")
	if strings.Contains(raw, id) {
		t.Fatal("a profile says what they like")
	}
	for range 2 {
		if code, _ := send(t, "DELETE", e.url+savePath(id), bob.token, nil, &st); code != http.StatusOK || st.Saved || st.Saves != 1 {
			t.Fatalf("unlike: %d %+v", code, st)
		}
	}
	if code, _ := send(t, "PUT", e.url+savePath("nosuchitem"), bob.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("no such item: %d", code)
	}
	review := e.publish(e.signIn(4, "dan", ""), publishParams("Waiting"), `<p>x</p>`)
	if code, _ := send(t, "PUT", e.url+savePath(review.ID), bob.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("one in review: %d", code)
	}
	// Taken back, it leaves the list; the limit is per account, and per minute.
	send(t, "PUT", e.url+savePath(id), bob.token, nil, nil)
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, id), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", code)
	}
	send(t, "GET", e.url+api.PathLibrarySaves, bob.token, nil, &mine)
	if len(mine.Items) != 0 {
		t.Fatalf("after it's gone: %v", listIDs(mine.Items))
	}
	other := e.liveAs(ada, "Other")
	limited := false
	for range 70 {
		if code, _ := send(t, "PUT", e.url+savePath(other), carol.token, nil, nil); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("no limit on likes")
	}
	if code, _ := send(t, "PUT", e.url+savePath(other), bob.token, nil, nil); code != http.StatusOK && code != http.StatusTooManyRequests {
		t.Fatalf("another account: %d", code)
	}
}

// A private collection is its owner's alone: to anyone else it isn't there, moderators
// included. A public one shows, and only its owner changes it.
func TestLibraryCollections(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	boss := e.webSession(e.admin.acc)
	id := e.liveAs(ada, "Shown")
	web := e.webSession(bob.acc)
	var private, public api.LibraryCollection
	if code, msg := send(t, "POST", e.url+api.PathLibraryCollections, web, api.LibraryCollectionParams{Title: "Secret plans", Description: "Mine"}, &private); code != http.StatusOK {
		t.Fatalf("make: %d %s", code, msg)
	}
	if code, msg := send(t, "POST", e.url+api.PathLibraryCollections, bob.token, api.LibraryCollectionParams{Title: "Good cards", Public: true}, &public); code != http.StatusOK {
		t.Fatalf("make: %d %s", code, msg)
	}
	for name, p := range map[string]api.LibraryCollectionParams{"no name": {Title: "  "}, "email": {Title: "Mail bob@example.com"}} {
		if code, _ := send(t, "POST", e.url+api.PathLibraryCollections, bob.token, p, nil); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	for _, c := range []string{private.ID, public.ID} {
		if code, msg := send(t, "PUT", e.url+collectionItemPath(c, id), bob.token, nil, nil); code != http.StatusNoContent {
			t.Fatalf("collect: %d %s", code, msg)
		}
	}
	privatePath, publicPath := itemPath(api.PathLibraryCollection, private.ID), itemPath(api.PathLibraryCollection, public.ID)
	for who, token := range map[string]string{"signed out": "", "someone": ada.token, "a moderator": boss} {
		if code, _ := send(t, "GET", e.url+privatePath, token, nil, nil); code != http.StatusNotFound {
			t.Errorf("private, %s: %d", who, code)
		}
		var page api.LibraryCollectionPage
		if code, _ := send(t, "GET", e.url+publicPath, token, nil, &page); code != http.StatusOK || len(page.Items) != 1 || page.Own {
			t.Errorf("public, %s: %d %+v", who, code, page)
		}
		var theirs api.LibraryCollections
		send(t, "GET", e.url+creatorPath(api.PathLibraryCreatorCollections, "bob"), token, nil, &theirs)
		if len(theirs.Collections) != 1 || theirs.Collections[0].ID != public.ID {
			t.Errorf("listed, %s: %+v", who, theirs)
		}
		raw, _, _ := doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "bob"), token)
		if strings.Contains(raw, "Secret plans") {
			t.Errorf("a profile says it, %s", who)
		}
	}
	var page api.LibraryCollectionPage
	if code, _ := send(t, "GET", e.url+privatePath, web, nil, &page); code != http.StatusOK || !page.Own || len(page.Items) != 1 || page.Collection.Items != 1 {
		t.Fatalf("its owner: %d %+v", code, page)
	}
	var mine api.LibraryCollections
	send(t, "GET", e.url+api.PathLibraryCollections+"?item="+id, bob.token, nil, &mine)
	if len(mine.Collections) != 2 || !mine.Collections[0].Contains || !mine.Collections[1].Contains {
		t.Fatalf("own list: %+v", mine)
	}
	var others api.LibraryCollections
	send(t, "GET", e.url+api.PathLibraryCollections, ada.token, nil, &others)
	if len(others.Collections) != 0 {
		t.Fatalf("someone else's list: %+v", others)
	}
	// Only its owner changes it; to anyone else it isn't there either.
	for name, call := range map[string]func(token string) int{
		"edit": func(token string) int {
			c, _ := send(t, "PUT", e.url+publicPath, token, api.LibraryCollectionParams{Title: "Mine now"}, nil)
			return c
		},
		"delete": func(token string) int { c, _ := send(t, "DELETE", e.url+publicPath, token, nil, nil); return c },
		"add": func(token string) int {
			c, _ := send(t, "PUT", e.url+collectionItemPath(public.ID, id), token, nil, nil)
			return c
		},
		"take out": func(token string) int {
			c, _ := send(t, "DELETE", e.url+collectionItemPath(public.ID, id), token, nil, nil)
			return c
		},
	} {
		if code := call(ada.token); code != http.StatusNotFound {
			t.Errorf("%s by someone else: %d", name, code)
		}
	}
	// Made private, it goes dark; a design taken back leaves it; deleting it asks nothing of the design.
	if code, _ := send(t, "PUT", e.url+publicPath, bob.token, api.LibraryCollectionParams{Title: "Good cards", Public: false}, nil); code != http.StatusNoContent {
		t.Fatalf("edit: %d", code)
	}
	if code, _ := send(t, "GET", e.url+publicPath, ada.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("made private: %d", code)
	}
	if code, _ := send(t, "DELETE", e.url+collectionItemPath(public.ID, id), bob.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("take out: %d", code)
	}
	send(t, "GET", e.url+publicPath, bob.token, nil, &page)
	if len(page.Items) != 0 {
		t.Fatalf("taken out: %+v", page)
	}
	if code, _ := send(t, "DELETE", e.url+publicPath, bob.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := send(t, "GET", e.url+itemPath(api.PathLibraryItem, id), "", nil, nil); code != http.StatusOK {
		t.Fatalf("the design after its collection went: %d", code)
	}
	if code, _ := send(t, "DELETE", e.url+itemPath(api.PathLibraryItem, id), ada.token, nil, nil); code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", code)
	}
	send(t, "GET", e.url+privatePath, bob.token, nil, &page)
	if len(page.Items) != 0 {
		t.Fatalf("after it was taken back: %+v", page)
	}
	if code, _ := send(t, "PUT", e.url+collectionItemPath(private.ID, id), bob.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("collecting one taken back: %d", code)
	}
}

// Following: the feed holds what those followed made; who follows whom is the
// follower's alone, a number being all anyone else gets.
func TestLibraryFollowAndFeed(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada, bob, carol := e.signIn(1, "ada", ""), e.signIn(2, "bob", ""), e.signIn(3, "carol", "")
	followPath := creatorPath(api.PathLibraryFollow, "ada")
	if code, _ := send(t, "PUT", e.url+followPath, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
	if _, err := e.db.LibraryProfile(context.Background(), ada.acc.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := send(t, "PUT", e.url+followPath, ada.token, nil, nil); code != http.StatusBadRequest {
		t.Fatalf("following oneself: %d", code)
	}
	if code, _ := send(t, "PUT", e.url+creatorPath(api.PathLibraryFollow, "nobody"), bob.token, nil, nil); code != http.StatusNotFound {
		t.Fatalf("no such creator: %d", code)
	}
	var st api.LibraryFollowState
	web := e.webSession(bob.acc)
	for _, token := range []string{bob.token, web} {
		if code, msg := send(t, "PUT", e.url+followPath, token, nil, &st); code != http.StatusOK || !st.Following || st.Followers != 1 {
			t.Fatalf("follow: %d %s %+v", code, msg, st)
		}
	}
	first := e.liveAs(ada, "First")
	second := e.liveAs(ada, "Second")
	e.liveAs(carol, "Not followed")
	var feed api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryFeed+"?limit=1", web, nil, &feed)
	if len(feed.Items) != 1 || feed.Items[0].ID != second || feed.Next == "" {
		t.Fatalf("feed: %+v", feed)
	}
	var nextPage api.LibraryItems
	send(t, "GET", e.url+api.PathLibraryFeed+"?limit=1&cursor="+feed.Next, web, nil, &nextPage)
	if len(nextPage.Items) != 1 || nextPage.Items[0].ID != first || nextPage.Next != "" {
		t.Fatalf("its next page: %+v", nextPage)
	}
	if code, _ := send(t, "GET", e.url+api.PathLibraryFeed, "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("a feed signed out: %d", code)
	}
	send(t, "GET", e.url+api.PathLibraryFeed, carol.token, nil, &feed)
	if len(feed.Items) != 0 {
		t.Fatalf("someone else's feed: %v", listIDs(feed.Items))
	}
	var who api.LibraryFollowing
	send(t, "GET", e.url+api.PathLibraryFollowing, bob.token, nil, &who)
	if len(who.Creators) != 1 || who.Creators[0].Handle != "ada" {
		t.Fatalf("following: %+v", who)
	}
	send(t, "GET", e.url+api.PathLibraryFollowing, carol.token, nil, &who)
	if len(who.Creators) != 0 {
		t.Fatalf("carol follows nobody: %+v", who)
	}
	_, seen := e.profile("ada", bob.token)
	if seen.Viewer == nil || !seen.Viewer.Following || seen.Stats.Followers != 1 {
		t.Fatalf("a profile, to who follows: %+v", seen)
	}
	raw, _, _ := doRaw(t, "GET", e.url+creatorPath(api.PathLibraryCreator, "ada"), carol.token)
	if strings.Contains(raw, "bob") || strings.Contains(raw, bob.acc.ID) {
		t.Fatalf("a profile names who follows: %s", raw)
	}
	if ids, err := e.db.LibraryFollowers(context.Background(), ada.acc.ID); err != nil || !slices.Equal(ids, []string{bob.acc.ID}) {
		t.Fatalf("followers, for the community's use: %v %v", ids, err)
	}
	for range 2 {
		if code, _ := send(t, "DELETE", e.url+followPath, bob.token, nil, &st); code != http.StatusOK || st.Following || st.Followers != 0 {
			t.Fatalf("unfollow: %d %+v", code, st)
		}
	}
	send(t, "GET", e.url+api.PathLibraryFeed, bob.token, nil, &feed)
	if len(feed.Items) != 0 {
		t.Fatalf("after leaving: %v", listIDs(feed.Items))
	}
}

// Deleting a profile keeps the Cloud account, so nothing cascades: what they wrote
// and were told goes by name. Export holds it first.
func TestLibraryDeleteTakesCommentsAndNotifications(t *testing.T) {
	e := newLibEnv(t, Config{})
	ctx := context.Background()
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	sub := e.publish(ada, publishParams("Mine"), `<p>x</p>`)
	e.decide(boss, sub.ID, 1, api.DecisionApprove, "ok")
	bob := e.signIn(2, "bob", "")
	if code, msg := send(t, "POST", e.url+itemPath(api.PathLibraryComments, sub.ID), bob.token, api.LibraryCommentParams{Body: "bob's words"}, nil); code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("comment: %d %s", code, msg)
	}
	var out api.LibraryExport
	if code, msg := send(t, "GET", e.url+api.PathLibraryExport, bob.token, nil, &out); code != http.StatusOK {
		t.Fatalf("export: %d %s", code, msg)
	}
	if len(out.Comments) != 1 || out.Comments[0].Body != "bob's words" {
		t.Fatalf("export's comments: %+v", out.Comments)
	}
	if code, msg := send(t, "GET", e.url+api.PathLibraryExport, ada.token, nil, &out); code != http.StatusOK || len(out.Notifications) == 0 {
		t.Fatalf("export's notifications: %d %s %+v", code, msg, out.Notifications)
	}
	for _, m := range []string{bob.acc.ID, ada.acc.ID} {
		if err := e.db.DeleteLibraryAccount(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{`SELECT COUNT(*) FROM library_comments`, `SELECT COUNT(*) FROM library_notifications`} {
		var n int
		if err := e.db.QueryRowContext(ctx, q).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d %v", q, n, err)
		}
	}
	if _, err := e.db.Account(ctx, bob.acc.ID); err != nil {
		t.Fatalf("the Cloud account went too: %v", err)
	}
}
