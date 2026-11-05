package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

// Opt in with an installed Codex executable. No credentials are used: its only
// provider is a closed localhost port, and all of its files live in the fixture.
func TestDeliveryNativeCodex(t *testing.T) {
	bin := os.Getenv("ENOWX_TEST_CODEX")
	if bin == "" {
		t.Skip("set ENOWX_TEST_CODEX for the isolated native regression")
	}
	for _, mode := range []string{"once", "draft", "menu"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			os.MkdirAll(home, 0700)
			codexHome := filepath.Join(root, "codex")
			os.MkdirAll(codexHome, 0700)
			srv := startServer(t)
			c := connect(t, srv, nil)
			info, err := c.CreateSession(proto.SessionCreateParams{Dir: home, Cols: 110, Rows: 32, Env: []string{"HOME=" + home, "USERPROFILE=" + home, "ENOWX_HOME=" + filepath.Join(root, "enowx"), "CODEX_HOME=" + codexHome, "LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"), "APPDATA=" + filepath.Join(home, "AppData", "Roaming")}, Command: []string{bin, "--no-daemon", "--no-alt-screen", "--dangerously-bypass-approvals-and-sandbox", "-c", `model_provider="fixture"`, "-c", `model="fixture"`, "-c", `model_providers.fixture={name="fixture",base_url="http://127.0.0.1:1/v1",wire_api="responses",requires_openai_auth=false,request_max_retries=0,stream_max_retries=0}`}})
			if err != nil {
				t.Fatal(err)
			}
			sess, _ := srv.sessions.Get(info.ID)
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("native screen: %s", sess.Terminal().Text())
				}
			})
			waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), "Ask Codex to do anything") })
			// Avoid mistaking initial drawing for a working agent.
			waitDelivery(t, func() bool { return !sess.Info().Active })
			p := proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "native", Text: "NATIVE_DELIVERY_ONCE"}
			if mode != "once" {
				text := "MANUAL_DRAFT"
				if mode == "menu" {
					text = "/permissions"
				}
				srv.sessions.Input(sess, []byte(text))
				if mode == "menu" {
					time.Sleep(300 * time.Millisecond)
					srv.sessions.Input(sess, []byte("\r"))
				}
				waitDelivery(t, func() bool {
					screen := sess.Terminal().Text()
					if mode == "draft" {
						return strings.Contains(screen, text)
					}
					return strings.Contains(screen, "Full Access") || strings.Contains(screen, "Full access") || strings.Contains(screen, "Read Only") || strings.Contains(screen, "Read-only")
				})
				t.Logf("%s screen: %s", mode, sess.Terminal().Text())
				if _, err := c.Deliver(p); err == nil {
					t.Fatal("typed into a manual draft or menu")
				}
				if len(nativeUserMessages(codexHome)) != 0 || strings.Contains(sess.Terminal().Text(), p.Text) {
					t.Fatal("touched the draft/menu")
				}
				return
			}
			if r, err := c.Deliver(p); err != nil || r.Interrupted {
				t.Fatalf("native send: %+v %v", r, err)
			}
			for range 5 {
				if _, err := c.Deliver(p); err != nil {
					t.Fatal(err)
				}
			}
			waitDelivery(t, func() bool { return len(nativeUserMessages(codexHome)) > 0 })
			if got := nativeUserMessages(codexHome); len(got) != 1 || got[0] != p.Text {
				t.Fatalf("recorded user messages: %q", got)
			}
			t.Logf("Codex recorded exactly one full UserMessage after delivery and five retries: %q", nativeUserMessages(codexHome))
			p.Key, p.Text = "busy", "BUSY_NOT_PASTED"
			if _, err := c.Deliver(p); err == nil {
				t.Fatal("pasted into the working native agent")
			}
		})
	}
}

func nativeUserMessages(home string) []string {
	var out []string
	filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(b), "\n") {
			var r struct {
				Type    string
				Payload struct {
					Type string
					Item struct {
						Type    string
						Content []struct{ Text string }
					}
				}
			}
			if json.Unmarshal([]byte(line), &r) == nil && r.Type == "event_msg" && r.Payload.Type == "item_completed" && r.Payload.Item.Type == "UserMessage" {
				var text strings.Builder
				for _, c := range r.Payload.Item.Content {
					text.WriteString(c.Text)
				}
				out = append(out, text.String())
			}
		}
		return nil
	})
	return out
}
