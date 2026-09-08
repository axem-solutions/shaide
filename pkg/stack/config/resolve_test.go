package config

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// recordingPrompter answers from a script and records what it was asked, so a
// test can assert which questions an operator actually sees.
type recordingPrompter struct {
	answers map[string]string
	asked   []string
}

func (p *recordingPrompter) Input(title, _, defaultValue string) (string, error) {
	p.asked = append(p.asked, title)
	if answer, ok := p.answers[title]; ok {
		return answer, nil
	}
	return defaultValue, nil
}

func (p *recordingPrompter) Select(title, current string, _ []string) (string, error) {
	p.asked = append(p.asked, title)
	if answer, ok := p.answers[title]; ok {
		return answer, nil
	}
	return current, nil
}

func (p *recordingPrompter) MultiSelect(title string, _ []string) ([]string, error) {
	p.asked = append(p.asked, title)
	return nil, nil
}

type testValues struct{}

func entry(key Key, source Source, prompt *Prompt, policy Policy) Entry[testValues] {
	return Entry[testValues]{Key: key, Source: source, Prompt: prompt, Policy: policy}
}

func TestResolvePrecedence(t *testing.T) {
	tests := []struct {
		name    string
		entry   Entry[testValues]
		want    string
		present bool
		asked   bool
	}{
		{
			// A runtime-known value must never be asked for.
			name:    "value wins over prompt and default",
			entry:   entry("k", Source{Value: "from-runtime", Default: "from-default"}, &Prompt{Kind: PromptInput, Title: "k?"}, Policy{}),
			want:    "from-runtime",
			present: true,
		},
		{
			// The default seeds the prompt rather than short-circuiting it.
			name:    "prompt wins over default",
			entry:   entry("k", Source{Default: "from-default"}, &Prompt{Kind: PromptInput, Title: "k?"}, Policy{}),
			want:    "from-default",
			present: true,
			asked:   true,
		},
		{
			name:    "default applies when nothing else does",
			entry:   entry("k", Source{Default: "from-default"}, nil, Policy{}),
			want:    "from-default",
			present: true,
		},
		{
			// Unset entries are omitted so a value written by hand into the
			// stack file is not overwritten with an empty one.
			name:    "unset entry is not written",
			entry:   entry("k", Source{}, nil, Policy{}),
			present: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompter := &recordingPrompter{}
			cfg := Config[testValues]{Namespace: "ns", Entries: []Entry[testValues]{test.entry}}

			got, err := cfg.Resolve(prompter)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			value, ok := got["ns:k"]
			if ok != test.present {
				t.Fatalf("present = %v, want %v (map %v)", ok, test.present, got)
			}
			if ok && value.Value != test.want {
				t.Errorf("value = %q, want %q", value.Value, test.want)
			}
			if asked := len(prompter.asked) > 0; asked != test.asked {
				t.Errorf("asked = %v, want %v", asked, test.asked)
			}
		})
	}
}

// A conditional entry must not be resolved, and must not be asked about, when
// its condition does not hold.
func TestResolveSkipsInactiveEntries(t *testing.T) {
	definition := Config[testValues]{
		Namespace: "ns",
		Entries: []Entry[testValues]{
			entry("platform", Source{Value: "gcp"}, nil, Policy{}),
			entry("albName", Source{}, &Prompt{Kind: PromptInput, Title: "ALB name"}, Policy{
				When: WhenEquals("platform", "azure"),
			}),
		},
	}

	prompter := &recordingPrompter{}
	got, err := definition.Resolve(prompter)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if _, ok := got["ns:albName"]; ok {
		t.Error("an inactive entry was written to the config map")
	}
	if len(prompter.asked) != 0 {
		t.Errorf("operator was asked %v for a platform that does not use it", prompter.asked)
	}
}

func TestResolveActivatesMatchingCondition(t *testing.T) {
	definition := Config[testValues]{
		Namespace: "ns",
		Entries: []Entry[testValues]{
			entry("platform", Source{Value: "azure"}, nil, Policy{}),
			entry("albName", Source{}, &Prompt{Kind: PromptInput, Title: "ALB name"}, Policy{
				When: WhenEquals("platform", "azure"),
			}),
		},
	}

	prompter := &recordingPrompter{answers: map[string]string{"ALB name": "shared-alb"}}
	got, err := definition.Resolve(prompter)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if got["ns:albName"].Value != "shared-alb" {
		t.Errorf("albName = %q, want the answered value", got["ns:albName"].Value)
	}
}

func TestResolveRequiredAndSecret(t *testing.T) {
	t.Run("required and missing fails", func(t *testing.T) {
		definition := Config[testValues]{
			Namespace: "ns",
			Entries:   []Entry[testValues]{entry("k", Source{}, nil, Policy{Required: true})},
		}

		if _, err := definition.Resolve(&recordingPrompter{}); err == nil {
			t.Fatal("Resolve() succeeded with a required entry unset")
		} else if !strings.Contains(err.Error(), "ns:k is required") {
			t.Errorf("error = %v, want it to name the key", err)
		}
	})

	t.Run("required and blank fails", func(t *testing.T) {
		definition := Config[testValues]{
			Namespace: "ns",
			Entries:   []Entry[testValues]{entry("k", Source{Value: "  "}, nil, Policy{Required: true})},
		}

		if _, err := definition.Resolve(&recordingPrompter{}); err == nil {
			t.Fatal("Resolve() accepted a blank value for a required entry")
		}
	})

	t.Run("secret is marked", func(t *testing.T) {
		definition := Config[testValues]{
			Namespace: "ns",
			Entries:   []Entry[testValues]{entry("k", Source{Value: "hunter2"}, nil, Policy{Secret: true})},
		}

		got, err := definition.Resolve(&recordingPrompter{})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if !got["ns:k"].Secret {
			t.Error("a secret entry was written without the secret flag")
		}
	})
}

// A condition may only depend on an entry declared earlier, because Resolve
// evaluates in order and would otherwise silently read a missing value.
func TestValidateRejectsForwardDependency(t *testing.T) {
	definition := Config[testValues]{
		Namespace: "ns",
		Entries: []Entry[testValues]{
			entry("albName", Source{Value: "x"}, nil, Policy{When: WhenEquals("platform", "azure")}),
			entry("platform", Source{Value: "azure"}, nil, Policy{}),
		},
	}

	if err := definition.Validate(); err == nil {
		t.Fatal("Validate() accepted a condition on a later entry")
	}
}

func TestValidateRejectsDuplicatesAndEmptyNamespace(t *testing.T) {
	duplicate := Config[testValues]{
		Namespace: "ns",
		Entries: []Entry[testValues]{
			entry("k", Source{Value: "a"}, nil, Policy{}),
			entry("k", Source{Value: "b"}, nil, Policy{}),
		},
	}
	if err := duplicate.Validate(); err == nil {
		t.Error("Validate() accepted a duplicate key")
	}

	noNamespace := Config[testValues]{Entries: []Entry[testValues]{entry("k", Source{Value: "a"}, nil, Policy{})}}
	if err := noNamespace.Validate(); err == nil {
		t.Error("Validate() accepted an empty namespace")
	}
}

func TestResolveEncodesNonStrings(t *testing.T) {
	definition := Config[testValues]{
		Namespace: "ns",
		Entries: []Entry[testValues]{
			entry("flag", Source{Value: true}, nil, Policy{}),
			entry("count", Source{Value: 3}, nil, Policy{}),
			entry("list", Source{Value: []string{"a", "b"}}, nil, Policy{}),
		},
	}

	got, err := definition.Resolve(&recordingPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := auto.ConfigMap{
		"ns:flag":  {Value: "true"},
		"ns:count": {Value: "3"},
		"ns:list":  {Value: `["a","b"]`},
	}
	for key, expected := range want {
		if got[key].Value != expected.Value {
			t.Errorf("%s = %q, want %q", key, got[key].Value, expected.Value)
		}
	}
}
