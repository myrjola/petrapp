package errorrecorder

import (
	"testing"
	"time"
)

func TestFakeClock_AdvanceMovesNowForward(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	clk := newFakeClock(start)
	if got := clk.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	clk.Advance(5 * time.Minute)
	want := start.Add(5 * time.Minute)
	if got := clk.Now(); !got.Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", got, want)
	}
}

func TestRealClock_NowReturnsMonotonicTime(t *testing.T) {
	t.Parallel()
	clk := realClock{}
	t1 := clk.Now()
	t2 := clk.Now()
	if t2.Before(t1) {
		t.Fatalf("realClock.Now() went backwards: %v then %v", t1, t2)
	}
}
