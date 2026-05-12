package domain

import "time"

// minDeloadCadence is the smallest sensible block length. A length of 1 would
// make every week a deload, and a length of 0 disables the calculation;
// callers should treat both as "feature off".
const minDeloadCadence = 2

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
