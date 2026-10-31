package decide

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/enowx/enowx/internal/quietcmd"
	"github.com/enowx/enowx/proto"
)

// answer is what a judge said: its text, the tokens it took, its cost
// when its service says.
type answer struct {
	text   string
	tokens int
	cost   float64
	took   time.Duration // asking it, its turn in the queue left out
}

// providerURLs are the OpenAI-compatible addresses of providers' APIs
// (Anthropic's is its own).
var providerURLs = map[string]string{
	proto.ProviderOpenAI:     "https://api.openai.com/v1",
	proto.ProviderGoogle:     "https://generativelanguage.googleapis.com/v1beta/openai",
	proto.ProviderOpenRouter: "https://openrouter.ai/api/v1",
}

// call asks judge j, the instructions in system, the question in user.
func (e *Engine) call(ctx context.Context, j proto.Judge, system, user string) (answer, error) {
	switch j.Kind {
	case proto.JudgeAgent:
		return e.callAgent(ctx, j, system+"\n\n"+user)
	case proto.JudgeAPI:
		key, err := e.key(j.Cred)
		if err != nil {
			return answer{}, err
		}
		if j.Protocol == "anthropic" || j.Provider == proto.ProviderAnthropic && j.URL == "" {
			return callAnthropic(ctx, j.URL, key, j.Model, system, user)
		}
		url := j.URL
		if url == "" {
			url = providerURLs[j.Provider]
		}
		return callOpenAI(ctx, url, key, j.Model, system, user)
	case proto.JudgeLocal:
		return callOpenAI(ctx, j.URL, "", j.Model, system, user)
	}
	return answer{}, fmt.Errorf("decide: no judge kind %q", j.Kind)
}

func (e *Engine) key(cred string) (string, error) {
	if e.KeyOf == nil {
		return "", errors.New("decide: no vault to read " + cred + " from")
	}
	return e.KeyOf(cred)
}

// callAgent runs agent j's one-shot command with prompt, in a folder of
// its own (it reads no project), none of the user's tools, servers or
// hooks loaded where the agent lets them be left out.
func (e *Engine) callAgent(ctx context.Context, j proto.Judge, prompt string) (answer, error) {
	dir, err := os.MkdirTemp("", "enowx-judge-")
	if err != nil {
		return answer{}, err
	}
	defer os.RemoveAll(dir)
	var args []string
	switch j.Agent {
	case "claude":
		args = []string{"-p", "--safe-mode", "--strict-mcp-config", "--output-format", "json", "--tools", "", "--no-session-persistence"}
		if j.Model != "" {
			args = append(args, "--model", j.Model)
		}
	case "codex":
		args = []string{"exec", "--ephemeral", "--skip-git-repo-check", "-s", "read-only", "-c", `model_reasoning_effort="low"`, "--json", "-o", filepath.Join(dir, "answer.txt")}
		if j.Model != "" {
			args = append(args, "-m", j.Model)
		}
	case "gemini":
		args = []string{"-p"}
		if j.Model != "" {
			args = append([]string{"-m", j.Model}, args...)
		}
	default:
		return answer{}, fmt.Errorf("decide: %s can't judge", j.Agent)
	}
	args = append(args, prompt)
	run := e.Run
	if run == nil {
		run = runCommand
	}
	out, err := run(ctx, dir, j.Agent, args)
	if err != nil {
		return answer{}, err
	}
	switch j.Agent {
	case "claude":
		var r struct {
			Result   string  `json:"result"`
			IsError  bool    `json:"is_error"`
			Cost     float64 `json:"total_cost_usd"`
			RawUsage struct {
				In        int `json:"input_tokens"`
				Out       int `json:"output_tokens"`
				CacheRead int `json:"cache_read_input_tokens"`
				CacheMade int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(out, &r) != nil {
			return answer{}, fmt.Errorf("decide: claude answered no JSON: %s", short(string(out)))
		}
		if r.IsError {
			return answer{}, fmt.Errorf("decide: claude: %s", short(r.Result))
		}
		u := r.RawUsage
		return answer{text: r.Result, tokens: u.In + u.Out + u.CacheRead + u.CacheMade, cost: r.Cost}, nil
	case "codex":
		text, _ := os.ReadFile(filepath.Join(dir, "answer.txt"))
		a := answer{text: string(text)}
		for _, line := range bytes.Split(out, []byte("\n")) {
			var ev struct {
				Type  string `json:"type"`
				Usage struct {
					In  int `json:"input_tokens"`
					Out int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(line, &ev) == nil && ev.Type == "turn.completed" {
				a.tokens += ev.Usage.In + ev.Usage.Out
			}
		}
		if strings.TrimSpace(a.text) == "" {
			return a, fmt.Errorf("decide: codex gave no answer: %s", short(string(out)))
		}
		return a, nil
	}
	return answer{text: string(out)}, nil
}

// runCommand runs bin with args in dir, nothing on its stdin, and says
// what it printed; its error output on a failure.
func runCommand(ctx context.Context, dir, bin string, args []string) ([]byte, error) {
	cmd := quietcmd.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return out, fmt.Errorf("decide: %s took too long", bin)
	}
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("decide: %s: %v %s", bin, err, short(stderr.String()))
	}
	return out, nil
}

// callAnthropic asks Anthropic's Messages API, or another speaking it at
// base ("" for Anthropic's).
func callAnthropic(ctx context.Context, base, key, model, system, user string) (answer, error) {
	if base == "" {
		base = "https://api.anthropic.com"
	}
	base = strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	body := map[string]any{"model": model, "max_tokens": 1500, "system": system,
		"messages": []map[string]string{{"role": "user", "content": user}}}
	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	}
	hdr := map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}
	if err := postJSON(ctx, base+"/v1/messages", hdr, body, &r); err != nil {
		return answer{}, err
	}
	var b strings.Builder
	for _, c := range r.Content {
		b.WriteString(c.Text)
	}
	return answer{text: b.String(), tokens: r.Usage.In + r.Usage.Out}, nil
}

// callOpenAI asks an OpenAI-compatible chat completions API at base.
func callOpenAI(ctx context.Context, base, key, model, system, user string) (answer, error) {
	body := map[string]any{"model": model, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	hdr := map[string]string{}
	if key != "" {
		hdr["Authorization"] = "Bearer " + key
	}
	if err := postJSON(ctx, strings.TrimRight(base, "/")+"/chat/completions", hdr, body, &r); err != nil {
		return answer{}, err
	}
	if len(r.Choices) == 0 {
		return answer{}, errors.New("decide: the model gave no answer")
	}
	return answer{text: r.Choices[0].Message.Content, tokens: r.Usage.Total}, nil
}

func postJSON(ctx context.Context, url string, hdr map[string]string, body, out any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("decide: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("decide: %s: %s", resp.Status, short(string(raw)))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decide: the answer: %w", err)
	}
	return nil
}

// short is s cut for a message.
func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
