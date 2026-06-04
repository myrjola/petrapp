package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestSignal_Label(t *testing.T) {
	t.Parallel()

	tests := []struct {
		signal domain.Signal
		want   string
	}{
		{domain.SignalTooHeavy, "too heavy"},
		{domain.SignalTooLight, "too light"},
		{domain.SignalOnTarget, ""},
		{domain.Signal("unknown"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.signal), func(t *testing.T) {
			t.Parallel()
			if got := tt.signal.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignal_Glyph(t *testing.T) {
	t.Parallel()

	tests := []struct {
		signal domain.Signal
		want   string
	}{
		{domain.SignalTooHeavy, "↓"},
		{domain.SignalTooLight, "↑"},
		{domain.SignalOnTarget, ""},
		{domain.Signal("unknown"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.signal), func(t *testing.T) {
			t.Parallel()
			if got := tt.signal.Glyph(); got != tt.want {
				t.Errorf("Glyph() = %q, want %q", got, tt.want)
			}
		})
	}
}
