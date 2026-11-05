package daemon

import (
	"testing"
	"time"
)

func TestDeliveryWriteCompletionOrCancellation(t *testing.T) {
	for _, kind := range []string{"written", "closed", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			written, stopped := make(chan struct{}), make(chan struct{})
			expired := make(chan time.Time, 1)
			result := make(chan bool, 1)
			go func() { result <- deliveryWritten(written, stopped, expired) }()
			select {
			case <-result:
				t.Fatal("advanced before a write or cancellation")
			default:
			}
			switch kind {
			case "written":
				close(written)
			case "closed":
				close(stopped)
			case "deadline":
				expired <- time.Now()
			}
			select {
			case got := <-result:
				if got != (kind == "written") {
					t.Fatalf("write outcome: %v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("write wait stranded")
			}
		})
	}
}
