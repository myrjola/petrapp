package domain

import (
	"math"
	"time"
)

// minDeloadCadence is the smallest sensible block length. A length of 1 would
// make every week a deload, and a length of 0 disables the calculation;
// callers should treat both as "feature off".
const minDeloadCadence = 2

// deloadAndZeroIndexOffset converts a 1-based block length to the 0-based index
// of the last training week: one step back for zero-indexing and one for the
// trailing deload week (block-week indices 0..length-2 train, length-1 deloads).
const deloadAndZeroIndexOffset = 2

const (
	// baseWeeklySets is the per-exercise working-set count in the first
	// training week of a mesocycle (and the floor the deload reduction works
	// from); peakWeeklySets is the count in the last training week before the
	// deload. The set-count ramp interpolates between them across the block. The
	// gap is deliberately small (+1): session length already bounds the number
	// of exercises, so one extra set per exercise is a meaningful week-over-week
	// set-count increase. Decoupled from PeriodizationType — the mesocycle week,
	// not Strength/Hypertrophy, drives set count (Phase D).
	baseWeeklySets = 3
	peakWeeklySets = 4
)

// WeekInBlock returns the 0-based week index within the mesocycle for date,
// given a Monday anchor and a block length. Dates strictly before the anchor
// return 0 (treated as "the anchor week starts in the future, count it as
// week 0 for now"). The calculation truncates to whole weeks; intra-week
// dates resolve to the same week as their Monday.
func WeekInBlock(date, anchor time.Time, length int) int {
	if length < minDeloadCadence || anchor.IsZero() {
		return 0
	}
	dayDiff := int(date.Sub(anchor).Hours() / 24)
	if dayDiff < 0 {
		return 0
	}
	weeks := dayDiff / 7
	return weeks % length
}

// IsDeloadWeek reports whether the date falls on the last (deload) week of
// its mesocycle. Returns false when the feature is disabled, when length is
// below minDeloadCadence, or when the anchor is the zero time.
func IsDeloadWeek(date, anchor time.Time, length int, enabled bool) bool {
	if !enabled || length < minDeloadCadence || anchor.IsZero() {
		return false
	}
	return WeekInBlock(date, anchor, length) == length-1
}

// MesocycleRampProgress returns the volume-ramp position in [0,1] for date
// within its mesocycle: 0 in the first training week, 1 in the last training
// week before the deload. It returns 0 whenever the mesocycle/deload feature is
// off (deload disabled, length below minDeloadCadence, or a zero anchor) and on
// the deload week itself, so scoring collapses to the static MinSets floor (the
// Phase B behaviour). Training weeks are the block-week indices 0..length-2; the
// deload week is length-1. Built on WeekInBlock so the 0-based block-week index
// is derived in exactly one place.
func MesocycleRampProgress(date, anchor time.Time, length int, deloadEnabled bool) float64 {
	if !deloadEnabled || length < minDeloadCadence || anchor.IsZero() {
		return 0
	}
	if IsDeloadWeek(date, anchor, length, deloadEnabled) {
		return 0
	}
	lastTrainingIdx := length - deloadAndZeroIndexOffset // 0..length-2 train; length-1 deloads.
	if lastTrainingIdx <= 0 {
		return 0 // single training week: no room to ramp.
	}
	week := min(WeekInBlock(date, anchor, length), lastTrainingIdx)
	return float64(week) / float64(lastTrainingIdx)
}

// SetsForWeek returns the base per-exercise working-set count for a session in
// date's mesocycle week, before any deload reduction: baseWeeklySets ramping to
// peakWeeklySets across the training weeks (see MesocycleRampProgress). The
// deload week and all feature-off states return baseWeeklySets; the deload -1
// reduction is applied downstream by deriveSchemeForExercise. The ramp is
// rounded to the nearest whole set.
func SetsForWeek(date, anchor time.Time, length int, deloadEnabled bool) int {
	progress := MesocycleRampProgress(date, anchor, length, deloadEnabled)
	return baseWeeklySets + int(math.Round(progress*float64(peakWeeklySets-baseWeeklySets)))
}
