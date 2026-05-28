package main

import (
	"strings"
	"testing"
)

func TestStripRepGuidanceLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantHas   []string
		wantNotIn []string
	}{
		{
			name: "drops 'perform 8-12 reps' from Instructions",
			input: "## Instructions\n" +
				"1. Set up the bar.\n" +
				"2. Lower with control.\n" +
				"3. Perform 8-12 reps.\n",
			wantHas:   []string{"Set up the bar", "Lower with control"},
			wantNotIn: []string{"Perform 8-12 reps", "8-12 reps"},
		},
		{
			name:      "drops 'complete 3 sets'",
			input:     "## Instructions\n1. Step one.\n2. Complete 3 sets of the movement.\n",
			wantHas:   []string{"Step one"},
			wantNotIn: []string{"Complete 3 sets"},
		},
		{
			name:      "drops 'hold for 30 seconds'",
			input:     "## Instructions\n1. Set up.\n2. Hold for 30 seconds at the bottom.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Hold for 30 seconds"},
		},
		{
			name:      "drops 'do 10 repetitions'",
			input:     "## Instructions\n1. Set up.\n2. Do 10 repetitions per side.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Do 10 repetitions"},
		},
		{
			name:      "keeps 'Take 2 deep breaths' — not a rep mention",
			input:     "## Instructions\n1. Take 2 deep breaths before lifting.\n",
			wantHas:   []string{"Take 2 deep breaths"},
			wantNotIn: []string{},
		},
		{
			name:      "keeps '3-second tempo' — not a rep mention",
			input:     "## Instructions\n1. Lower at a 3-second tempo.\n",
			wantHas:   []string{"3-second tempo"},
			wantNotIn: []string{},
		},
		{
			name:      "drops bare 'repetition guidance' from literal template leak",
			input:     "## Instructions\n1. Set up.\n2. Optional step 5 with repetition guidance.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"repetition guidance"},
		},
		{
			name: "leaves Common Mistakes alone (would be irregular hit otherwise)",
			input: "## Instructions\n1. Set up.\n\n## Common Mistakes\n" +
				"- Doing 50 reps at once: pace yourself.\n",
			wantHas:   []string{"Doing 50 reps at once"},
			wantNotIn: []string{},
		},
		{
			name:      "drops 'aim for 8-12 reps' (fixture form)",
			input:     "## Instructions\n1. Set up.\n2. Aim for 8-12 reps, switch sides.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Aim for 8-12 reps", "8-12 reps"},
		},
		{
			name:      "drops 'perform 3 sets of 10-15 repetitions' (fixture form)",
			input:     "## Instructions\n1. Set up.\n2. Perform 3 sets of 10-15 repetitions for activation.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Perform 3 sets of 10-15 repetitions"},
		},
		{
			name:      "drops 'start with 3 sets of 5-8 controlled reps' (fixture form)",
			input:     "## Instructions\n1. Set up.\n2. Start with 3 sets of 5-8 controlled reps using a manageable weight.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Start with 3 sets of 5-8 controlled reps"},
		},
		{
			name:      "drops 'aim for 3 sets of 8-12 repetitions' (fixture form)",
			input:     "## Instructions\n1. Set up.\n2. Aim for 3 sets of 8-12 repetitions, ensuring controlled form.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Aim for 3 sets of 8-12 repetitions"},
		},
		{
			name:      "drops 'aim for 8-12 controlled reps' (fixture compound form)",
			input:     "## Instructions\n1. Set up.\n2. Start with light weight and aim for 8-12 controlled reps.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"aim for 8-12 controlled reps"},
		},
		{
			name:      "drops 'do 3 sets' (symmetry with 'do N reps')",
			input:     "## Instructions\n1. Set up.\n2. Do 3 sets of slow controlled movement.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Do 3 sets"},
		},
		{
			name:      "drops 'hold this position for 30 seconds' (fixture form)",
			input:     "## Instructions\n1. Set up.\n2. Hold this position for 30 seconds at the bottom.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Hold this position for 30 seconds"},
		},
		{
			name:      "keeps 'Start with your legs together' — start without sets phrase",
			input:     "## Instructions\n1. Start with your legs together and slightly in front.\n",
			wantHas:   []string{"Start with your legs together"},
			wantNotIn: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := StripRepGuidanceLines(tt.input)
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

func TestExtractResourceURLs(t *testing.T) {
	t.Parallel()

	desc := "## Instructions\n1. Set up.\n\n## Resources\n" +
		"- [A](https://a.example.org/x)\n" +
		"- [B](http://b.example.org/y)\n" +
		"- [C without link]\n" +
		"- [D](https://a.example.org/x)\n" // duplicate

	got := ExtractResourceURLs(desc)

	want := []string{
		"https://a.example.org/x",
		"http://b.example.org/y",
		"https://a.example.org/x", // ExtractResourceURLs preserves duplicates; caller dedupes
	}
	if len(got) != len(want) {
		t.Fatalf("got %d URLs, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("URL[%d] = %q, want %q", i, got[i], w)
		}
	}
}
