package main

import (
	"fmt"
	"testing"
)

func Test_formatFloat(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{62.54, "62.5"},               // weight progression artifact (0.85 factor)
		{60.900000000000006, "60.9"},  // original floating-point artifact
		{75.0, "75"},                  // whole number, no trailing zero
		{0.0, "0"},                    // zero
		{100.0, "100"},                // larger whole number
		{62.5, "62.5"},               // already at one decimal place
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
			if got := formatFloat(tt.input); got != tt.want {
				t.Errorf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
