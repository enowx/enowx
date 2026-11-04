package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/store/storetest"
	"github.com/enowx/enowx/core/designshare"
	"github.com/enowx/enowx/proto"
)

// publishFrame publishes page in frame, with picture, and says how it stands.
func (e *libEnv) publishFrame(m signedInMember, p api.LibraryPublishParams, f proto.CanvasFrame, picture []byte) api.LibrarySubmitted {
	e.t.Helper()
	var made api.LibraryPublished
	if code, msg := send(e.t, "POST", e.url+api.PathLibraryItems, m.token, p, &made); code != http.StatusOK {
		e.t.Fatalf("create: %d %s", code, msg)
	}
	data, err := designshare.Build(f, nil, nil).Marshal()
	if err != nil {
		e.t.Fatal(err)
	}
	if code, msg := e.put(m.token, partPath(made.ID, made.Version, api.PartSource), data); code != http.StatusNoContent {
		e.t.Fatalf("source: %d %s", code, msg)
	}
	if code, msg := e.put(m.token, partPath(made.ID, made.Version, api.PartThumb), picture); code != http.StatusNoContent {
		e.t.Fatalf("thumb: %d %s", code, msg)
	}
	var out api.LibrarySubmitted
	if code, msg := send(e.t, "POST", e.url+strings.NewReplacer("{id}", made.ID, "{version}", string(rune('0'+made.Version))).Replace(api.PathLibrarySubmit), m.token, nil, &out); code != http.StatusOK {
		e.t.Fatalf("submit: %d %s", code, msg)
	}
	return out
}

// trusted signs someone in whose first design was approved, so what they publish
// goes live at once.
func (e *libEnv) trusted(gh int64, login string) signedInMember {
	e.t.Helper()
	m := e.signIn(gh, login, login+"@example.com")
	p := publishParams("Their first " + login)
	p.Description, p.Tags = "zzzfirst", nil
	sub := e.publish(m, p, "<p>first</p>")
	if code, msg := e.decide(e.admin.token, sub.ID, 1, api.DecisionApprove, "fine"); code != http.StatusNoContent {
		e.t.Fatalf("approve: %d %s", code, msg)
	}
	return m
}

// titles are the titles a query of the public list answers, in order.
func (e *libEnv) titles(query string) []string {
	e.t.Helper()
	var out api.LibraryItems
	if code, msg := send(e.t, "GET", e.url+api.PathLibraryItems+"?"+query, "", nil, &out); code != http.StatusOK {
		e.t.Fatalf("%s: %d %s", query, code, msg)
	}
	titles := []string{}
	for _, it := range out.Items {
		titles = append(titles, it.Title)
	}
	return titles
}

func onPostgres() bool { return os.Getenv(storetest.Env) != "" }

// A search finds by title, tags and description, the best matches first, unless
// another order is asked for. Both databases.
func TestLibrarySearchRanksAndKeepsTheAPI(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.trusted(1, "ada")
	publish := func(title, about string, tags ...string) {
		p := publishParams(title)
		p.Description, p.Tags = about, tags
		if s := e.publish(ada, p, "<p>x</p>"); s.Status != api.LibraryLive {
			t.Fatalf("%s: %s", title, s.Status)
		}
	}
	publish("Harbor board", "Berths and cranes")
	publish("Crane tags", "Nothing else", "harbor", "cargo-ship")
	publish("Tide table", "For the harbor master")
	publish("Pricing", "Three plans")

	if got := e.titles("q=harbor"); !slices.Equal(got, []string{"Harbor board", "Crane tags", "Tide table"}) {
		t.Fatalf("ranked: %v", got)
	}
	if got := e.titles("q=harbor&sort=relevant"); got[0] != "Harbor board" {
		t.Fatalf("asked for ranking: %v", got)
	}
	if got := e.titles("q=harbor&sort=new"); !slices.Equal(got, []string{"Tide table", "Crane tags", "Harbor board"}) {
		t.Fatalf("newest: %v", got)
	}
	// The last word may still be being typed; all words have to be there.
	if got := e.titles("q=harb"); len(got) != 3 {
		t.Fatalf("a prefix: %v", got)
	}
	if got := e.titles("q=" + url.QueryEscape("berths harb")); !slices.Equal(got, []string{"Harbor board"}) {
		t.Fatalf("two words: %v", got)
	}
	if got := e.titles("q=cargo"); !slices.Equal(got, []string{"Crane tags"}) {
		t.Fatalf("part of a tag: %v", got)
	}
	if got := e.titles("q=nothinglikeit"); len(got) != 0 {
		t.Fatalf("no match: %v", got)
	}
	// What a query language would read as an operator is only punctuation here.
	for _, q := range []string{"harbor & | ! ( ) : *", "'; DROP TABLE library_items; --", `\`, "%", "_", "!!!", "a:*b", "(", strings.Repeat("harbor ", 40)} {
		if code, msg := send(t, "GET", e.url+api.PathLibraryItems+"?q="+url.QueryEscape(q), "", nil, nil); code != http.StatusOK {
			t.Fatalf("%q: %d %s", q, code, msg)
		}
	}
	if got := e.titles("q=" + url.QueryEscape("'; DROP TABLE library_items; --")); len(got) != 0 {
		t.Fatalf("punctuation found: %v", got)
	}
	if got := e.titles("q=harbor"); len(got) != 3 {
		t.Fatalf("after all of that: %v", got)
	}
	// A wrong cursor still answers as it did.
	if code, _ := send(t, "GET", e.url+api.PathLibraryItems+"?q=harbor&cursor=!!", "", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("cursor: %d", code)
	}
}

// On Postgres, a word's forms match, and the search goes through its index.
func TestLibrarySearchOnPostgres(t *testing.T) {
	if !onPostgres() {
		t.Skip("needs " + storetest.Env)
	}
	e := newLibEnv(t, Config{})
	ada := e.trusted(1, "ada")
	p := publishParams("Dashboards for shipping")
	p.Description, p.Tags = "Boards", nil
	e.publish(ada, p, "<p>x</p>")
	if got := e.titles("q=dashboard"); !slices.Equal(got, []string{"Dashboards for shipping"}) {
		t.Fatalf("a word's form: %v", got)
	}
	ctx := context.Background()
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.QueryContext(ctx, `EXPLAIN SELECT id FROM library_items i WHERE i.search @@ to_tsquery('english', 'dashboard:*')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		rows.Scan(&line)
		plan.WriteString(line + "\n")
	}
	if !strings.Contains(plan.String(), "library_items_search") {
		t.Fatalf("not through the index:\n%s", plan.String())
	}
}

// Filters: what the creator picked, and what the server read from the page itself,
// whatever the host said of it.
func TestLibraryFilters(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada := e.trusted(1, "ada")
	picture := thumb(t)
	send1 := func(title, style string, width int, motion bool, html string) {
		p := publishParams(title)
		p.Style, p.HasMotion, p.Width = style, motion, width
		p.Theme, p.Device = "light", "desktop" // a host's word, not taken
		if s := e.publishFrame(ada, p, proto.CanvasFrame{Name: "Home", Width: width, Height: 600, HTML: html}, picture); s.Status != api.LibraryLive {
			t.Fatalf("%s: %s", title, s.Status)
		}
	}
	send1("Night", "minimal", 1200, false, `<style>body{background:#0b0d14}.b{background:#2c5fd4}</style><button class="b">Go</button>`)
	send1("Day", "editorial", 1200, true, `<style>body{background:#f4f0e8}.b{background:#c8481f}</style><button class="b">Go</button>`)
	send1("Pocket", "not-a-style", 390, false, `<body style="background:#2f7d5b"><p>x</p></body>`)

	for query, want := range map[string][]string{
		"color=blue":                  {"Night"},
		"color=red":                   {"Day"},
		"color=green":                 {"Pocket"},
		"color=paper":                 {"Day", "Their first ada"}, // a page saying nothing is white
		"theme=dark":                  {"Night", "Pocket"},
		"device=phone":                {"Pocket"},
		"style=minimal":               {"Night"},
		"style=editorial&color=paper": {"Day"},
		"style=editorial&color=blue":  {},
		"motion=1":                    {"Day"},
		"motion=1&theme=dark":         {},
		"license=mit&color=blue":      {"Night"},
		"category=dashboard&q=night&color=blue&theme=dark&device=desktop&style=minimal&kind=page": {"Night"},
	} {
		got := e.titles(query)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s: %v, wants %v", query, got, want)
		}
	}
	if got := e.titles("device=desktop"); slices.Contains(got, "Pocket") || !slices.Contains(got, "Night") {
		t.Errorf("desktop: %v", got)
	}

	// A filter's value is only ever a parameter: it finds nothing, and changes nothing.
	for _, name := range []string{"style", "color", "theme", "device", "category", "kind", "license", "tag"} {
		for _, bad := range []string{"' OR '1'='1", "x'; DROP TABLE library_items; --", `") OR 1=1 --`} {
			query := name + "=" + url.QueryEscape(bad)
			if got := e.titles(query); len(got) != 0 {
				t.Errorf("%s found %v", query, got)
			}
		}
	}
	if got := e.titles("color=blue"); !slices.Equal(got, []string{"Night"}) {
		t.Errorf("after those: %v", got)
	}

	// A wide design with a phone version counts as made for phones too.
	data, err := designshare.Build(proto.CanvasFrame{Name: "Wide", Width: 1200, Height: 600, HTML: "<p>x</p>"}, &proto.CanvasFrame{Name: "Narrow", Width: 390, Height: 600, HTML: "<p>x</p>"}, nil).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var made api.LibraryPublished
	send(t, "POST", e.url+api.PathLibraryItems, ada.token, publishParams("Both"), &made)
	e.put(ada.token, partPath(made.ID, 1, api.PartSource), data)
	e.put(ada.token, partPath(made.ID, 1, api.PartThumb), picture)
	if code, msg := send(t, "POST", e.url+strings.NewReplacer("{id}", made.ID, "{version}", "1").Replace(api.PathLibrarySubmit), ada.token, nil, nil); code != http.StatusOK {
		t.Fatalf("submit: %d %s", code, msg)
	}
	if got := e.titles("device=phone"); !slices.Contains(got, "Both") {
		t.Errorf("a phone version: %v", got)
	}
	// Each version keeps its own colors.
	colors, err := e.db.LibraryVersionColors(context.Background(), made.ID, 1)
	if err != nil || len(colors) == 0 || colors[0].Color != "paper" {
		t.Errorf("colors: %+v, %v", colors, err)
	}
	var it api.LibraryItem
	send(t, "GET", e.url+itemPath(api.PathLibraryItem, made.ID), "", nil, &it)
	if it.Theme != "light" || it.Device != "desktop" {
		t.Errorf("item: %+v", it)
	}
}

// The colors the filters offer are what the server finds.
func TestLibraryColorsAreKnown(t *testing.T) {
	for _, hex := range []string{"#c8481f", "#e3b23c", "#2f7d5b", "#1f7a8c", "#2c5fd4", "#3d2fa0", "#1c1a22", "#f4f0e8"} {
		look := designshare.LookOf(designshare.Frame{Width: 800, Background: hex})
		if len(look.Colors) != 1 || !slices.Contains(api.LibraryColors, look.Colors[0].Name) {
			t.Errorf("%s reads as %+v", hex, look.Colors)
		}
	}
}

// Searches are limited per address, the browsers behind the website too, and
// going over stops only searching.
func TestLibrarySearchIsLimitedPerAddress(t *testing.T) {
	e := newLibEnv(t, Config{})
	get := func(path string, headers map[string]string) int {
		code, _ := sendWith(t, "GET", e.url+path, "", headers, nil, nil)
		return code
	}
	over := func(headers map[string]string) int {
		for i := 0; i < 70; i++ {
			if code := get(api.PathLibraryItems+"?q=word", headers); code != http.StatusOK {
				return code
			}
		}
		return http.StatusOK
	}
	if code := over(nil); code != http.StatusTooManyRequests {
		t.Fatalf("over the limit: %d", code)
	}
	if code := get(api.PathLibraryItems, nil); code != http.StatusOK {
		t.Fatalf("plain browsing: %d", code)
	}
	web := func(addr string) map[string]string {
		return map[string]string{api.HeaderLibraryWeb: libSecret, "X-Forwarded-For": addr}
	}
	if code := get(api.PathLibraryItems+"?q=word", web("203.0.113.1")); code != http.StatusOK {
		t.Fatalf("someone else, through the website: %d", code)
	}
	if code := over(web("203.0.113.2")); code != http.StatusTooManyRequests {
		t.Fatalf("one browser through the website: %d", code)
	}
}
