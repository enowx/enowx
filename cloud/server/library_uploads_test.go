package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/cloud/library"
)

// movingItem is a new design of m's that may take a video, and that video.
func (e *libEnv) movingItem(m signedInMember, title string) (api.LibraryPublished, []byte) {
	e.t.Helper()
	p := publishParams(title)
	p.HasMotion = true
	var made api.LibraryPublished
	if code, msg := send(e.t, "POST", e.url+api.PathLibraryItems, m.token, p, &made); code != http.StatusOK {
		e.t.Fatalf("create: %d %s", code, msg)
	}
	return made, mp4(5, "vide")
}

// One account sending the same part again and again runs into its own limit, long
// before the month's writes are spent for everyone.
func TestLibraryPutsHaveALimitPerAccount(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	made, video := e.movingItem(ada, "Mine")
	limited := false
	for range 100 {
		code, _ := e.put(ada.token, partPath(made.ID, 1, api.PartVideo), video)
		if code == http.StatusTooManyRequests {
			limited = true
			break
		}
		if code != http.StatusNoContent {
			t.Fatalf("a put: %d", code)
		}
	}
	if !limited {
		t.Fatal("no limit in 100 puts")
	}
	other, video := e.movingItem(bob, "Theirs")
	if code, msg := e.put(bob.token, partPath(other.ID, 1, api.PartVideo), video); code != http.StatusNoContent {
		t.Fatalf("another account: %d %s", code, msg)
	}
}

// Bytes written in a day are an account's too, and what's refused isn't counted
// against the bucket.
func TestLibraryWritesHaveADayPerAccount(t *testing.T) {
	video := mp4(5, "vide")
	e := newLibEnvWith(t, func(c *library.Config) { c.AccountWrittenDay = int64(len(video)) * 3 })
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	made, _ := e.movingItem(ada, "Mine")
	path := partPath(made.ID, 1, api.PartVideo)
	for range 3 {
		if code, msg := e.put(ada.token, path, video); code != http.StatusNoContent {
			t.Fatalf("within the day: %d %s", code, msg)
		}
	}
	if code, _ := e.put(ada.token, path, video); code != http.StatusTooManyRequests {
		t.Fatalf("past the day: %d", code)
	}
	boss := e.webSession(e.admin.acc)
	var u api.LibraryUsage
	send(t, "GET", e.url+api.PathLibraryUsage, boss, nil, &u)
	if u.BytesWritten != int64(len(video))*3 {
		t.Fatalf("written: %d", u.BytesWritten)
	}
	other, _ := e.movingItem(bob, "Theirs")
	if code, msg := e.put(bob.token, partPath(other.ID, 1, api.PartVideo), video); code != http.StatusNoContent {
		t.Fatalf("another account: %d %s", code, msg)
	}
}

// Bodies held open take an account's own places, not everyone's.
func TestLibraryPutsAtOnceArePerAccount(t *testing.T) {
	e := newLibEnv(t, Config{})
	ada, bob := e.signIn(1, "ada", ""), e.signIn(2, "bob", "")
	made, video := e.movingItem(ada, "Mine")
	path := partPath(made.ID, 1, api.PartVideo)
	for range 2 {
		body, hold := io.Pipe()
		t.Cleanup(func() { hold.Close() })
		req, _ := http.NewRequest("PUT", e.url+path, body)
		req.Header.Set("Authorization", "Bearer "+ada.token)
		go func() {
			if res, err := http.DefaultClient.Do(req); err == nil {
				res.Body.Close()
			}
		}()
		hold.Write([]byte("x")) // its handler is reading from here on
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, _ := e.put(ada.token, path, video)
		if code == http.StatusTooManyRequests {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a third at once: %d", code)
		}
		time.Sleep(20 * time.Millisecond)
	}
	other, _ := e.movingItem(bob, "Theirs")
	if code, msg := e.put(bob.token, partPath(other.ID, 1, api.PartVideo), video); code != http.StatusNoContent {
		t.Fatalf("another account: %d %s", code, msg)
	}
}

// A blocked account's upload stops where it stands.
func TestLibraryPutsRefuseABlockedAccount(t *testing.T) {
	e := newLibEnv(t, Config{})
	boss := e.webSession(e.admin.acc)
	ada := e.signIn(1, "ada", "")
	made, video := e.movingItem(ada, "Mine")
	if code, msg := send(t, "POST", e.url+strings.NewReplacer("{id}", ada.acc.ID).Replace(api.PathLibraryBlock), boss, api.LibraryReason{Reason: "spam"}, nil); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, msg)
	}
	if code, _ := e.put(ada.token, partPath(made.ID, 1, api.PartVideo), video); code != http.StatusForbidden {
		t.Fatalf("a put: %d", code)
	}
}
