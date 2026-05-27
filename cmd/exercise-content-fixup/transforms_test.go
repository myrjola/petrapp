package main

import (
	"strings"
	"testing"
)

func TestStripDeadResourceLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		alive     map[string]bool
		wantHas   []string
		wantNotIn []string
	}{
		{
			name: "drops example.com placeholders unconditionally",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [Video](https://example.com/v)\n" +
				"- [Guide](https://example.com/g)\n",
			alive:     map[string]bool{},
			wantNotIn: []string{"## Resources", "example.com"},
			wantHas:   []string{"## Instructions"},
		},
		{
			name: "drops dead URLs, keeps live",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [Live](https://live.example.org/a)\n" +
				"- [Dead](https://dead.example.org/b)\n",
			alive:     map[string]bool{"https://live.example.org/a": true},
			wantHas:   []string{"## Resources", "[Live]", "## Instructions"},
			wantNotIn: []string{"[Dead]"},
		},
		{
			name: "drops Resources heading when nothing survives",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [A](https://dead.example.org/a)\n" +
				"- [B](https://dead.example.org/b)\n",
			alive:     map[string]bool{},
			wantNotIn: []string{"## Resources", "[A]", "[B]"},
			wantHas:   []string{"## Instructions"},
		},
		{
			name:      "no Resources section is a no-op",
			input:     "## Instructions\n1. Step\n\n## Common Mistakes\n- Bad form\n",
			alive:     map[string]bool{},
			wantHas:   []string{"## Instructions", "## Common Mistakes", "- Bad form"},
			wantNotIn: []string{"## Resources"},
		},
		{
			name: "Resources at end of file with no trailing section",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [Live](https://live.example.org/a)\n",
			alive:     map[string]bool{"https://live.example.org/a": true},
			wantHas:   []string{"## Resources", "[Live]"},
			wantNotIn: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := StripDeadResourceLinks(tt.input, tt.alive)
			for _, want := range tt.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q; got:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotIn {
				if strings.Contains(got, notWant) {
					t.Errorf("found %q but should be absent; got:\n%s", notWant, got)
				}
			}
		})
	}
}
