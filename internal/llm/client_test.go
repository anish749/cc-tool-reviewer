package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errBackend = errors.New("backend failed")

func TestText(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		genErr  error
		block   bool // generate blocks until ctx is cancelled
		timeout time.Duration
		want    string
		wantErr error
	}{
		{name: "trims whitespace", raw: "  hello  \n", want: "hello"},
		{name: "propagates backend error", genErr: errBackend, wantErr: errBackend},
		{name: "applies timeout around the call", block: true, timeout: 20 * time.Millisecond, wantErr: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &client{
				timeout: tt.timeout,
				generate: func(ctx context.Context, _, _ string) (string, error) {
					if tt.block {
						<-ctx.Done()
						return "", ctx.Err()
					}
					return tt.raw, tt.genErr
				},
			}
			got, err := c.Text(context.Background(), "sys", "prompt")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Text = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		genErr   error
		want     string // decoded out["k"] when a clean decode is expected
		parseErr bool   // expect a *ParseError
		wantErr  error  // expect errors.Is(err, wantErr) — a transport failure
	}{
		{name: "unmarshals object", raw: `{"k":"v"}`, want: "v"},
		{name: "strips code fences", raw: "```json\n{\"k\":\"v\"}\n```", want: "v"},
		{name: "malformed reply is a ParseError", raw: "not json at all", parseErr: true},
		{name: "propagates backend error", genErr: errBackend, wantErr: errBackend},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &client{
				generate: func(context.Context, string, string) (string, error) {
					return tt.raw, tt.genErr
				},
			}
			var out map[string]string
			err := c.JSON(context.Background(), "sys", "prompt", &out)

			switch {
			case tt.parseErr:
				var perr *ParseError
				if !errors.As(err, &perr) {
					t.Fatalf("expected *ParseError, got %T: %v", err, err)
				}
				if perr.Raw != tt.raw {
					t.Fatalf("ParseError.Raw = %q, want %q", perr.Raw, tt.raw)
				}
			case tt.wantErr != nil:
				var perr *ParseError
				if errors.As(err, &perr) {
					t.Fatalf("transport error misclassified as ParseError: %v", err)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if out["k"] != tt.want {
					t.Fatalf("decoded k = %q, want %q", out["k"], tt.want)
				}
			}
		})
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "plain", "plain"},
		{"json fence", "```json\n{}\n```", "{}"},
		{"bare fence", "```\n{}\n```", "{}"},
		{"inline fence with spaces", "  ```json {} ```", "{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCodeFences(tt.in); got != tt.want {
				t.Errorf("stripCodeFences(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
