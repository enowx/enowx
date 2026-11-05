package daemon

import (
	"strings"
	"testing"

	"github.com/enowx/enowx/proto"
)

// A hook can announce a permission wait without any intervening terminal input.
func TestReviewDeliveryStopsEnterWhenStateChanges(t *testing.T) {
	srv, c, info, path := deliveryFixture(t)
	sess, _ := srv.sessions.Get(info.ID)
	type answer struct {
		result proto.DeliveryResult
		err    error
	}
	done := make(chan answer, 1)
	go func() {
		r, err := c.Deliver(proto.DeliveryParams{ID: info.ID, Run: info.Run, Key: "review-state-change", Text: "REVIEW_PENDING"})
		done <- answer{r, err}
	}()
	waitDelivery(t, func() bool { return strings.Contains(sess.Terminal().Text(), "REVIEW_PENDING") })
	srv.sessions.SetState(sess, proto.StateWaiting)
	a := <-done
	if a.err != nil {
		t.Fatal(a.err)
	}
	if !a.result.Interrupted || len(capturedLines(path)) != 0 {
		t.Fatalf("delayed Enter sent after state became waiting: result=%+v captured=%q", a.result, capturedLines(path))
	}
}
