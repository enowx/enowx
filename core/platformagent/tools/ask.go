package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/enowx/enowx/core/platformagent/record"
)

const askSchema = `{"type":"object","properties":{"questions":{"type":"array","minItems":1,"description":"The questions to ask","items":{"type":"object","properties":{"header":{"type":"string","description":"A label of a few words"},"question":{"type":"string","description":"The question, in full"},"options":{"type":"array","description":"The answers to pick from","items":{"type":"object","properties":{"label":{"type":"string","description":"The answer"},"description":{"type":"string","description":"What picking it means"}},"required":["label"],"additionalProperties":false}},"multiSelect":{"type":"boolean","description":"More than one answer may be picked"}},"required":["question"],"additionalProperties":false}}},"required":["questions"],"additionalProperties":false}`

type askTool struct{ asker Asker }

// NewAskUserQuestion is the tool that puts questions to the user through a.
func NewAskUserQuestion(a Asker) Tool { return askTool{asker: a} }

func (askTool) Name() string { return "AskUserQuestion" }
func (askTool) Description() string {
	return "Ask the user questions and wait for the answers, when a decision is theirs and you can't settle it from what you have."
}
func (askTool) Schema() json.RawMessage { return json.RawMessage(askSchema) }
func (askTool) ReadOnly() bool          { return true }
func (askTool) Edits() bool             { return false }

type askInput struct {
	Questions []struct {
		Header   string `json:"header"`
		Question string `json:"question"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"options"`
		Multi bool `json:"multiSelect"`
	} `json:"questions"`
}

func (in askInput) record() []record.Question {
	qs := make([]record.Question, 0, len(in.Questions))
	for _, q := range in.Questions {
		one := record.Question{Header: q.Header, Question: q.Question, Multi: q.Multi}
		for _, o := range q.Options {
			one.Options = append(one.Options, o.Label)
			one.About = append(one.About, o.Description)
		}
		qs = append(qs, one)
	}
	return qs
}

func (askTool) Detail(input json.RawMessage) string {
	in, err := decode[askInput](input)
	if err != nil || len(in.Questions) == 0 {
		return "AskUserQuestion"
	}
	return oneLine(in.Questions[0].Question, 200)
}

func (t askTool) Run(ctx context.Context, input json.RawMessage, _ Env) Result {
	in, err := decode[askInput](input)
	if err != nil {
		return failf("%v", err)
	}
	if len(in.Questions) == 0 {
		return failf("questions is required")
	}
	if t.asker == nil {
		return failf("no one is there to answer questions")
	}
	qs := in.record()
	answers, err := t.asker.Questions(ctx, qs)
	if err != nil {
		return failf("the questions weren't answered: %v", err)
	}
	var out strings.Builder
	for i, q := range qs {
		answer := "(no answer)"
		if i < len(answers) {
			answer = answers[i]
		}
		out.WriteString(q.Question + "\n→ " + answer + "\n")
	}
	return Result{Text: out.String()}
}
