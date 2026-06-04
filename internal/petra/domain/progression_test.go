package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestCurrentSet_FirstSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		periodization  domain.PeriodizationType
		repMin         int
		repMax         int
		startingWeight float64
		wantReps       int
		wantWeight     float64
	}{
		{
			name:           "strength returns 5 reps",
			periodization:  domain.PeriodizationStrength,
			repMin:         5,
			repMax:         10,
			startingWeight: 80.0,
			wantReps:       5,
			wantWeight:     80.0,
		},
		{
			name:           "hypertrophy returns 8 reps",
			periodization:  domain.PeriodizationHypertrophy,
			repMin:         5,
			repMax:         8,
			startingWeight: 60.0,
			wantReps:       8,
			wantWeight:     60.0,
		},
		{
			name:           "zero starting weight is returned as-is",
			periodization:  domain.PeriodizationHypertrophy,
			repMin:         5,
			repMax:         8,
			startingWeight: 0.0,
			wantReps:       8,
			wantWeight:     0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := domain.NewProgression(domain.Config{
				Type:           tt.periodization,
				RepMin:         tt.repMin,
				RepMax:         tt.repMax,
				StartingWeight: tt.startingWeight,
				IsDeload:       false,
			})
			got := p.CurrentSet()
			if got.TargetReps != tt.wantReps {
				t.Errorf("TargetReps = %d, want %d", got.TargetReps, tt.wantReps)
			}
			if got.WeightKg != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got.WeightKg, tt.wantWeight)
			}
		})
	}
}

func TestCurrentSet_SignalAdjustment(t *testing.T) {
	t.Parallel()

	const startWeight = 100.0

	tests := []struct {
		name       string
		signal     domain.Signal
		wantWeight float64
	}{
		{
			name:       "TooLight increases by 2.5kg",
			signal:     domain.SignalTooLight,
			wantWeight: 102.5,
		},
		{
			name:       "TooHeavy decreases by max(2.5kg, 10%)",
			signal:     domain.SignalTooHeavy,
			wantWeight: 90.0, // |w|*0.10 = 10kg > 2.5kg minimum → 100 - 10 = 90.0
		},
		{
			name:       "OnTarget keeps same weight",
			signal:     domain.SignalOnTarget,
			wantWeight: startWeight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := domain.NewProgression(domain.Config{
				Type:           domain.PeriodizationHypertrophy,
				RepMin:         5,
				RepMax:         8,
				StartingWeight: startWeight,
				IsDeload:       false,
			})
			p.RecordCompletion(domain.SetResult{
				ActualReps: 8,
				Signal:     tt.signal,
				WeightKg:   startWeight,
			})
			got := p.CurrentSet()
			if got.WeightKg != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got.WeightKg, tt.wantWeight)
			}
		})
	}
}

func TestCurrentSet_TooHeavyRounding(t *testing.T) {
	t.Parallel()

	// 23kg: |w|*0.10 = 2.3, below the 2.5kg minimum step → 23 - 2.5 = 20.5
	p := domain.NewProgression(domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         5,
		RepMax:         8,
		StartingWeight: 23.0,
		IsDeload:       false,
	})
	p.RecordCompletion(domain.SetResult{
		ActualReps: 5,
		Signal:     domain.SignalTooHeavy,
		WeightKg:   23.0,
	})
	got := p.CurrentSet()
	if got.WeightKg != 20.5 {
		t.Errorf("WeightKg = %v, want 20.5", got.WeightKg)
	}
}

func TestCurrentSet_OverridePropagates(t *testing.T) {
	t.Parallel()

	// Recommended set 1 = 100kg. User overrides to 95kg and signals OnTarget.
	// Set 2 recommendation must be 95kg (from actual), not 100kg.
	p := domain.NewProgression(domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         5,
		RepMax:         8,
		StartingWeight: 100.0,
		IsDeload:       false,
	})
	p.RecordCompletion(domain.SetResult{
		ActualReps: 8,
		Signal:     domain.SignalOnTarget,
		WeightKg:   95.0, // user lifted less than recommended
	})
	got := p.CurrentSet()
	if got.WeightKg != 95.0 {
		t.Errorf("WeightKg = %v, want 95.0 (override weight)", got.WeightKg)
	}
}

func TestCurrentSet_OverrideThenTooLight(t *testing.T) {
	t.Parallel()

	// User overrides set 2 to 90kg and signals TooLight.
	// Set 3 must be 90 + 2.5 = 92.5kg.
	p := domain.NewProgression(domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         5,
		RepMax:         8,
		StartingWeight: 100.0,
		IsDeload:       false,
	})
	p.RecordCompletion(domain.SetResult{
		ActualReps: 8,
		Signal:     domain.SignalOnTarget,
		WeightKg:   100.0,
	})
	p.RecordCompletion(domain.SetResult{
		ActualReps: 8,
		Signal:     domain.SignalTooLight,
		WeightKg:   90.0, // user overrode set 2 down to 90kg
	})
	got := p.CurrentSet()
	if got.WeightKg != 92.5 {
		t.Errorf("WeightKg = %v, want 92.5", got.WeightKg)
	}
}

func TestNewFromHistory_MatchesReplay(t *testing.T) {
	t.Parallel()

	config := domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         5,
		RepMax:         8,
		StartingWeight: 80.0,
		IsDeload:       false,
	}
	results := []domain.SetResult{
		{ActualReps: 8, Signal: domain.SignalTooLight, WeightKg: 80.0},
		{ActualReps: 8, Signal: domain.SignalOnTarget, WeightKg: 82.5},
	}

	// Build via replay.
	replay := domain.NewProgression(config)
	for _, r := range results {
		replay.RecordCompletion(r)
	}

	// Build via NewFromHistory.
	history := domain.NewProgressionFromHistory(config, results)

	replayTarget := replay.CurrentSet()
	historyTarget := history.CurrentSet()

	if replayTarget != historyTarget {
		t.Errorf("NewFromHistory CurrentSet = %+v, want %+v", historyTarget, replayTarget)
	}
	if history.SetsCompleted() != replay.SetsCompleted() {
		t.Errorf("SetsCompleted = %d, want %d", history.SetsCompleted(), replay.SetsCompleted())
	}
}

func TestNewFromHistory_EmptySliceEqualsNew(t *testing.T) {
	t.Parallel()

	config := domain.Config{
		Type:           domain.PeriodizationStrength,
		RepMin:         5,
		RepMax:         10,
		StartingWeight: 60.0,
		IsDeload:       false,
	}
	fresh := domain.NewProgression(config)
	fromEmpty := domain.NewProgressionFromHistory(config, nil)

	if fresh.CurrentSet() != fromEmpty.CurrentSet() {
		t.Errorf("CurrentSet mismatch: fresh=%+v history=%+v", fresh.CurrentSet(), fromEmpty.CurrentSet())
	}
}

func TestSetsCompleted(t *testing.T) {
	t.Parallel()

	p := domain.NewProgression(domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         5,
		RepMax:         8,
		StartingWeight: 60.0,
		IsDeload:       false,
	})

	if p.SetsCompleted() != 0 {
		t.Errorf("SetsCompleted before any sets = %d, want 0", p.SetsCompleted())
	}

	p.RecordCompletion(domain.SetResult{
		ActualReps: 8,
		Signal:     domain.SignalOnTarget,
		WeightKg:   60.0,
	})
	if p.SetsCompleted() != 1 {
		t.Errorf("SetsCompleted after 1 set = %d, want 1", p.SetsCompleted())
	}

	p.RecordCompletion(domain.SetResult{
		ActualReps: 8,
		Signal:     domain.SignalTooLight,
		WeightKg:   60.0,
	})
	if p.SetsCompleted() != 2 {
		t.Errorf("SetsCompleted after 2 sets = %d, want 2", p.SetsCompleted())
	}
}

func TestAdjustedWeight_AssistedAndZeroBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		lastWeight float64
		signal     domain.Signal
		wantWeight float64
	}{
		{
			name:       "negative TooHeavy goes further negative (more assistance)",
			lastWeight: -20.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: -22.5,
		},
		{
			name:       "zero TooHeavy goes negative (zero boundary fixed)",
			lastWeight: 0.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: -1.0,
		},
		{
			name:       "negative TooLight goes less negative (less assistance)",
			lastWeight: -20.0,
			signal:     domain.SignalTooLight,
			wantWeight: -17.5,
		},
		{
			name:       "negative OnTarget holds steady",
			lastWeight: -20.0,
			signal:     domain.SignalOnTarget,
			wantWeight: -20.0,
		},
		{
			name:       "threshold weight TooHeavy uses 2.5kg increment and snaps to whole kg",
			lastWeight: 10.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: 8.0, // 10 - max(2.5, 1.0) = 7.5; below threshold, snaps to 8.
		},
		{
			name:       "high positive TooHeavy uses percentage (regression)",
			lastWeight: 100.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: 90.0,
		},
		{
			name:       "mid positive TooHeavy uses percentage (regression)",
			lastWeight: 50.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: 45.0,
		},
		{
			name:       "dumbbell-range TooLight increases by 1kg",
			lastWeight: 5.0,
			signal:     domain.SignalTooLight,
			wantWeight: 6.0,
		},
		{
			name:       "dumbbell-range TooLight at 9kg lands on 10kg threshold",
			lastWeight: 9.0,
			signal:     domain.SignalTooLight,
			wantWeight: 10.0,
		},
		{
			name:       "zero TooLight increases by 1kg",
			lastWeight: 0.0,
			signal:     domain.SignalTooLight,
			wantWeight: 1.0,
		},
		{
			name:       "dumbbell-range TooHeavy decreases by 1kg",
			lastWeight: 5.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: 4.0,
		},
		{
			name:       "dumbbell-range TooHeavy at 1kg lands on 0kg",
			lastWeight: 1.0,
			signal:     domain.SignalTooHeavy,
			wantWeight: 0.0,
		},
		{
			name:       "threshold TooLight crosses into 2.5kg increment",
			lastWeight: 10.0,
			signal:     domain.SignalTooLight,
			wantWeight: 12.5,
		},
		{
			name:       "off-grid override TooLight snaps to whole kg",
			lastWeight: 7.5, // user override; not a real fixed dumbbell.
			signal:     domain.SignalTooLight,
			wantWeight: 9.0, // 7.5 + 1 = 8.5 → snaps to 9.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := domain.NewProgressionFromHistory(
				domain.Config{
					Type:           domain.PeriodizationStrength,
					RepMin:         5,
					RepMax:         10,
					StartingWeight: 0,
					IsDeload:       false,
				},
				[]domain.SetResult{
					{ActualReps: 5, Signal: tt.signal, WeightKg: tt.lastWeight},
				},
			)
			got := p.CurrentSet().WeightKg
			if got != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got, tt.wantWeight)
			}
		})
	}
}

// TestExhaustivePeriodizationCoverage documents that every PeriodizationType
// resolves to a non-zero rep count via DeriveScheme. Adding a new variant without
// updating the switch in DeriveScheme will both fail this test and trip the
// `exhaustive` linter on the package's internal switches.
func TestExhaustivePeriodizationCoverage(t *testing.T) {
	t.Parallel()

	// Use a wide window so repMin/repMax don't mask any periodization branch.
	const repMin, repMax = 5, 15
	all := []domain.PeriodizationType{
		domain.PeriodizationStrength,
		domain.PeriodizationHypertrophy,
	}
	for _, p := range all {
		if got := domain.DeriveScheme(repMin, repMax, p, false).TargetReps; got <= 0 {
			t.Errorf("DeriveScheme(%d,%d,%v).TargetReps = %d, want positive", repMin, repMax, p, got)
		}
	}
}

func TestProgression_DeloadFirstSetUsesStartingWeight(t *testing.T) {
	t.Parallel()

	cfg := domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         8,
		RepMax:         12,
		StartingWeight: 67.5,
		IsDeload:       true,
	}
	p := domain.NewProgression(cfg)

	target := p.CurrentSet()
	if target.WeightKg != 67.5 {
		t.Errorf("initial CurrentSet WeightKg = %v, want 67.5", target.WeightKg)
	}
	if target.TargetReps != 12 {
		t.Errorf("initial CurrentSet TargetReps = %d, want 12 (hypertrophy → repMax)", target.TargetReps)
	}
}

// Deload sets are recorded with a nil signal (the form has only a single
// "Done!" button). Subsequent sets must echo whatever weight the user
// actually lifted on the previous set so a one-time correction (e.g. the
// rack only has 60 kg, not the seeded 61 kg) propagates forward without
// the user having to re-enter it for every remaining set.
func TestProgression_DeloadCarriesUserOverrideForward(t *testing.T) {
	t.Parallel()

	cfg := domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         8,
		RepMax:         12,
		StartingWeight: 61.0,
		IsDeload:       true,
	}
	p := domain.NewProgression(cfg)

	// User adjusts the seeded weight down to 60 kg and signals "Done!"
	// (deload submits no signal — the zero-value Signal stands in).
	p.RecordCompletion(domain.SetResult{
		ActualReps: 12,
		Signal:     "",
		WeightKg:   60.0,
	})
	if got := p.CurrentSet().WeightKg; got != 60.0 {
		t.Errorf("after override, deload CurrentSet WeightKg = %v, want 60.0", got)
	}

	// A second set at the same weight keeps the recommendation steady.
	p.RecordCompletion(domain.SetResult{
		ActualReps: 12,
		Signal:     "",
		WeightKg:   60.0,
	})
	if got := p.CurrentSet().WeightKg; got != 60.0 {
		t.Errorf("after second set, deload CurrentSet WeightKg = %v, want 60.0", got)
	}

	// Another override (e.g. the user takes a heavier dumbbell) propagates too.
	p.RecordCompletion(domain.SetResult{
		ActualReps: 12,
		Signal:     "",
		WeightKg:   62.5,
	})
	if got := p.CurrentSet().WeightKg; got != 62.5 {
		t.Errorf("after second override, deload CurrentSet WeightKg = %v, want 62.5", got)
	}
}

func TestDeloadSeedWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		working float64
		factor  float64
		want    float64
	}{
		// Weighted (positive): scale down, floor.
		{"weighted whole kg stays whole", 60.0, 0.9, 54.0},
		{"weighted 75 × 0.9 floors 67.5 → 67", 75.0, 0.9, 67.0},
		{"weighted 80 × 0.9 stays 72", 80.0, 0.9, 72.0},
		{"weighted 75 × 0.8 stays 60", 75.0, 0.8, 60.0},
		{"weighted 100.99 × 0.9 floors 90.89 → 90", 100.99, 0.9, 90.0},
		{"weighted zero stays zero", 0.0, 0.9, 0.0},
		// Assisted (negative): more assistance = larger magnitude. Scale
		// magnitude UP by 1/factor and ceil to whole kg, then re-apply sign.
		{"assisted -50 with 0.9 → -ceil(55.56) = -56", -50.0, 0.9, -56.0},
		{"assisted -50 with 0.8 → -ceil(62.5) = -63", -50.0, 0.8, -63.0},
		{"assisted -45 with 0.9 → -ceil(50.0) = -50", -45.0, 0.9, -50.0},
		{"assisted whole-kg result is preserved", -30.0, 1.0, -30.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.DeloadSeedWeight(tt.working, tt.factor); got != tt.want {
				t.Errorf("DeloadSeedWeight(%v, %v) = %v, want %v",
					tt.working, tt.factor, got, tt.want)
			}
		})
	}
}

func TestAdjustedWeight_UnknownSignalDoesNotPanic(t *testing.T) {
	t.Parallel()
	p := domain.NewProgressionFromHistory(
		domain.Config{Type: domain.PeriodizationStrength, RepMin: 5, RepMax: 8, StartingWeight: 50, IsDeload: false},
		[]domain.SetResult{{ActualReps: 5, Signal: domain.Signal("bogus"), WeightKg: 60}},
	)
	got := p.CurrentSet()
	if got.WeightKg != 60 {
		t.Errorf("unknown signal: got weight %v, want unchanged 60", got.WeightKg)
	}
}

// TestExhaustiveSignalCoverage documents that every valid Signal resolves to
// a finite weight via the package's internal adjustedWeight switch (exercised
// through CurrentSet).
func TestExhaustiveSignalCoverage(t *testing.T) {
	t.Parallel()

	valid := []domain.Signal{
		domain.SignalTooHeavy,
		domain.SignalOnTarget,
		domain.SignalTooLight,
	}
	for _, s := range valid {
		p := domain.NewProgressionFromHistory(
			domain.Config{
				Type:           domain.PeriodizationHypertrophy,
				RepMin:         5,
				RepMax:         8,
				StartingWeight: 50,
				IsDeload:       false,
			},
			[]domain.SetResult{
				{ActualReps: 8, Signal: s, WeightKg: 50},
			},
		)
		// The call would panic if the switch in adjustedWeight failed to
		// handle the signal; the assertion just guards against silent zeros.
		if got := p.CurrentSet().WeightKg; got == 0 && s != domain.SignalTooHeavy {
			t.Errorf("CurrentSet for signal %v unexpectedly returned 0", s)
		}
	}
}
