package main

import "testing"

func Test_sanitiseFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "absolute path", in: "/workouts/2026-05-24", want: "/workouts/2026-05-24"},
		{name: "absolute path with query", in: "/workouts/2026-05-24?x=1", want: "/workouts/2026-05-24?x=1"},
		{name: "protocol-relative URL rejected", in: "//evil.example.com/foo", want: ""},
		{name: "absolute http URL rejected", in: "http://evil.example.com/foo", want: ""},
		{name: "relative path without slash rejected", in: "workouts/2026-05-24", want: ""},
		{name: "double-slash inside path rejected", in: "/foo//bar", want: ""},
		{name: "javascript scheme rejected", in: "javascript:alert(1)", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitiseFromPath(tc.in); got != tc.want {
				t.Errorf("sanitiseFromPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
