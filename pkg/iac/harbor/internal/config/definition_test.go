package config

import (
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

// recordingPrompter answers every prompt with a fixed value and records the
// titles it was asked.
type recordingPrompter struct {
	titles []string
}

func (p *recordingPrompter) Input(title, _, _ string) (string, error) {
	p.titles = append(p.titles, title)
	return "answer", nil
}

func (p *recordingPrompter) Select(title, current string, _ []string) (string, error) {
	p.titles = append(p.titles, title)
	return current, nil
}

func (p *recordingPrompter) MultiSelect(title string, _ []string) ([]string, error) {
	p.titles = append(p.titles, title)
	return nil, nil
}

func TestNodeHostnameIsAskedOnlyOnPrem(t *testing.T) {
	tests := []struct {
		platform cluster.Provider
		want     bool
	}{
		{platform: cluster.OnPrem, want: true},
		{platform: cluster.AWS, want: false},
		{platform: cluster.GCP, want: false},
		{platform: cluster.Azure, want: false},
	}

	for _, tc := range tests {
		t.Run(string(tc.platform), func(t *testing.T) {
			prompter := &recordingPrompter{}
			definition := newDefinition(stack.Options{Platform: tc.platform, Kubeconfig: "/kubeconfig"})

			resolved, err := definition.Resolve(prompter)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			asked := false
			for _, title := range prompter.titles {
				if title == "Harbor storage node hostname" {
					asked = true
				}
			}
			if asked != tc.want {
				t.Errorf("node hostname asked = %v, want %v (prompts: %v)", asked, tc.want, prompter.titles)
			}

			_, set := resolved[definition.Key(KeyNodeHostname)]
			if set != tc.want {
				t.Errorf("%s set = %v, want %v", definition.Key(KeyNodeHostname), set, tc.want)
			}
		})
	}
}
