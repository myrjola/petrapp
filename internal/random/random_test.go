package random_test

import (
	"github.com/myrjola/petrapp/internal/random"
	"testing"
)

func TestRandomLetters(t *testing.T) {
	tests := []struct {
		name    string
		length  uint
		wantErr bool
	}{
		{
			name:    "zero length",
			length:  0,
			wantErr: false,
		},
		{
			name:    "32 length",
			length:  32,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := random.Letters(tt.length)
			if (err != nil) != tt.wantErr {
				t.Errorf("Letters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if uint(len(got)) != tt.length {
				t.Errorf("Letters() got length = %v, want length %v", len(got), tt.length)
			}
		})
	}
}
