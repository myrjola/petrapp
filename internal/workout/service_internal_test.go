package workout

import (
	"testing"
	"time"
)

func TestMondayOf_UsesLocalCalendarAnchoredToUTC(t *testing.T) {
	helsinki, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatalf("load Europe/Helsinki: %v", err)
	}

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			// Sunday 00:32 EEST: previously Truncate(24h) rolled the result
			// back into Sunday in local time.
			name: "early-morning Sunday in EEST returns previous Monday",
			in:   time.Date(2026, 5, 3, 0, 32, 41, 0, helsinki),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Monday after midnight EEST returns same Monday",
			in:   time.Date(2026, 5, 4, 0, 32, 0, 0, helsinki),
			want: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Monday afternoon UTC returns same Monday",
			in:   time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Tuesday 02:00 EEST returns same week's Monday",
			in:   time.Date(2026, 4, 28, 2, 0, 0, 0, helsinki),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Saturday late evening UTC returns same week's Monday",
			in:   time.Date(2026, 5, 2, 23, 59, 0, 0, time.UTC),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mondayOf(tt.in)
			if !got.Equal(tt.want) {
				t.Errorf("mondayOf(%s) = %s, want %s", tt.in, got, tt.want)
			}
			if got.Weekday() != time.Monday {
				t.Errorf("mondayOf(%s).Weekday() = %s, want Monday", tt.in, got.Weekday())
			}
		})
	}
}
