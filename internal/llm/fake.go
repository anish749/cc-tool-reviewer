package llm

import (
	"context"
	"encoding/json"
)

// Fake is a test double for Client, for packages that depend on llm. When Err is
// nil, JSON unmarshals Reply into out (mimicking a decoded reply); otherwise
// both Text and JSON return Err. It records the most recent prompts it received.
type Fake struct {
	Reply     string
	Err       error
	GotSystem string
	GotPrompt string
}

func (f *Fake) Text(_ context.Context, systemPrompt, prompt string) (string, error) {
	f.GotSystem, f.GotPrompt = systemPrompt, prompt
	return f.Reply, f.Err
}

func (f *Fake) JSON(_ context.Context, systemPrompt, prompt string, out any) error {
	f.GotSystem, f.GotPrompt = systemPrompt, prompt
	if f.Err != nil {
		return f.Err
	}
	return json.Unmarshal([]byte(f.Reply), out)
}
