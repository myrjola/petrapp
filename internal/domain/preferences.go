package domain

// Preferences stores how long a user wants to work out each day of the week.
// A value of 0 means rest day; any positive integer means workout day with
// that duration in minutes.
type Preferences struct {
	MondayMinutes    int
	TuesdayMinutes   int
	WednesdayMinutes int
	ThursdayMinutes  int
	FridayMinutes    int
	SaturdayMinutes  int
	SundayMinutes    int
}

func (p Preferences) Monday() bool    { return p.MondayMinutes > 0 }
func (p Preferences) Tuesday() bool   { return p.TuesdayMinutes > 0 }
func (p Preferences) Wednesday() bool { return p.WednesdayMinutes > 0 }
func (p Preferences) Thursday() bool  { return p.ThursdayMinutes > 0 }
func (p Preferences) Friday() bool    { return p.FridayMinutes > 0 }
func (p Preferences) Saturday() bool  { return p.SaturdayMinutes > 0 }
func (p Preferences) Sunday() bool    { return p.SundayMinutes > 0 }

// IsEmpty reports whether no workout days are scheduled.
func (p Preferences) IsEmpty() bool {
	return p.MondayMinutes == 0 && p.TuesdayMinutes == 0 && p.WednesdayMinutes == 0 &&
		p.ThursdayMinutes == 0 && p.FridayMinutes == 0 && p.SaturdayMinutes == 0 &&
		p.SundayMinutes == 0
}
