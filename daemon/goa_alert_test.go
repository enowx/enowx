package daemon

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/enowx/enowx/proto"
)

func TestGOARepeatedFlagsKeepAuditButLimitNotifications(t *testing.T) {
	srv := startServer(t)
	events := make(chan proto.Event, 64)
	connect(t, srv, func(e proto.Event) { events <- e })
	flag := proto.GOAFlag{Session: "goa", Command: "unknown.exe"}
	for range 5 {
		srv.goaFlag(flag)
	}
	srv.goaFlag(proto.GOAFlag{Session: "goa", Command: "another.exe"})
	srv.goaFlag(proto.GOAFlag{Session: "other-goa", Command: "unknown.exe"})
	// A quiet minute permits a fresh notification for the same activity.
	srv.goa.mu.Lock()
	for i := range srv.goa.flags {
		srv.goa.flags[i].Time = time.Now().Add(-2 * time.Minute)
	}
	srv.goa.mu.Unlock()
	srv.goaFlag(flag)
	srv.goaFlag(proto.GOAFlag{Session: "barrier", Command: "barrier"})
	flags, alerts := 0, 0
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-events:
			switch e.Name {
			case proto.EventGOAFlag:
				flags++
			case proto.EventNotifySound:
				var p proto.NotifySoundParams
				if err := json.Unmarshal(e.Data, &p); err != nil {
					t.Fatal(err)
				}
				alerts++
				if p.Session == "barrier" {
					if flags != 9 || alerts != 5 || len(srv.goaFlags()) != 9 {
						t.Fatalf("flags=%d alerts=%d kept=%d; want 9,5,9", flags, alerts, len(srv.goaFlags()))
					}
					return
				}
			}
		case <-deadline:
			t.Fatal("notification barrier never reached")
		}
	}
}
