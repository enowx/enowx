package mcpclient

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
)

// sseTransport is a server of type "sse", the older HTTP transport: one GET stream
// carries everything from the server, its first event naming where to POST messages.
//
// readLoop owns the stream; sends run on their callers' goroutines.
type sseTransport struct {
	cfg      serverConfig
	client   *http.Client
	endpoint string
	cancel   context.CancelFunc
	in       chan []byte
	over     chan struct{}
	whyText  string // set before over closes
	once     sync.Once
}

func dialSSE(ctx context.Context, cfg serverConfig) (*sseTransport, error) {
	// The stream outlives ctx, which bounds only getting it.
	streamCtx, cancel := context.WithCancel(context.Background())
	stopWatch := context.AfterFunc(ctx, cancel)
	defer stopWatch()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		cancel()
		return nil, plainError(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	t := &sseTransport{cfg: cfg, client: httpClient(), cancel: cancel, in: make(chan []byte), over: make(chan struct{})}
	resp, err := t.client.Do(req)
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, plainError(err)
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		cancel()
		return nil, errors.New(statusText(resp))
	}
	endpoint := make(chan string, 1)
	go t.readLoop(streamCtx, resp, endpoint)
	select {
	case e, ok := <-endpoint:
		if !ok {
			t.close()
			return nil, errors.New("its stream ended before it named where to send messages")
		}
		target, err := sameOrigin(cfg.URL, e)
		if err != nil {
			t.close()
			return nil, err
		}
		t.endpoint = target
		return t, nil
	case <-ctx.Done():
		t.close()
		return nil, ctx.Err()
	}
}

// sameOrigin resolves the endpoint a stream names, refusing another host: messages go
// with the server's headers, and they hold its keys.
func sameOrigin(base, endpoint string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", errors.New("its url can't be read")
	}
	u, err := b.Parse(endpoint)
	if err != nil || u.Scheme != b.Scheme || u.Host != b.Host {
		return "", errors.New("it names another host to send messages to")
	}
	return u.String(), nil
}

func (t *sseTransport) readLoop(ctx context.Context, resp *http.Response, endpoint chan<- string) {
	defer resp.Body.Close()
	named := false
	err := readEvents(resp.Body, func(event string, data []byte) bool {
		switch event {
		case "endpoint":
			if !named {
				named = true
				endpoint <- string(bytes.TrimSpace(data))
			}
		case "", "message":
			select {
			case t.in <- data:
			case <-ctx.Done():
				return false
			}
		}
		return true
	})
	if !named {
		close(endpoint)
	}
	t.whyText = "its stream ended"
	if err != nil && ctx.Err() == nil {
		t.whyText = "its stream ended: " + plainError(err).Error()
	}
	close(t.over)
}

func (t *sseTransport) send(ctx context.Context, msg []byte, _ int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(msg))
	if err != nil {
		return plainError(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return plainError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return errors.New(statusText(resp))
	}
	return nil
}

func (t *sseTransport) inbound() <-chan []byte { return t.in }
func (t *sseTransport) ended() <-chan struct{} { return t.over }
func (t *sseTransport) why() string            { return t.whyText }
func (t *sseTransport) negotiated(string)      {}

func (t *sseTransport) close() {
	t.once.Do(func() {
		t.cancel()
		<-t.over
	})
}
