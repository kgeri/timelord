// Package policy defines the policy primitives shared by the schedule model.
package policy

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// MinutesPerDay is the number of minutes in a day.
const MinutesPerDay = 24 * 60

// Day is a weekday name in a schedule.
type Day string

// Weekday names used in schedules.
const (
	Monday    Day = "mon"
	Tuesday   Day = "tue"
	Wednesday Day = "wed"
	Thursday  Day = "thu"
	Friday    Day = "fri"
	Saturday  Day = "sat"
	Sunday    Day = "sun"
)

// Valid reports whether d is a known weekday name.
func (d Day) Valid() bool {
	switch d {
	case Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday:
		return true
	default:
		return false
	}
}

// Clock is a time of day, stored as minutes since midnight.
type Clock int

// ParseClock parses a 24-hour HH:MM value.
func ParseClock(s string) (Clock, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("clock %q must use HH:MM", s)
	}
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("clock %q must use HH:MM", s)
		}
	}

	hour, _ := strconv.Atoi(s[:2])
	minute, _ := strconv.Atoi(s[3:])
	if hour > 23 || minute > 59 {
		return 0, fmt.Errorf("clock %q is out of range", s)
	}
	return Clock(hour*60 + minute), nil
}

// String returns the clock as HH:MM.
func (c Clock) String() string {
	return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60)
}

// Valid reports whether c is a time between 00:00 and 23:59.
func (c Clock) Valid() bool {
	return c >= 0 && c < MinutesPerDay
}

// UnmarshalJSON parses an HH:MM string.
func (c *Clock) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	clock, err := ParseClock(s)
	if err != nil {
		return err
	}
	*c = clock
	return nil
}

// MarshalJSON writes the clock as an HH:MM string.
func (c Clock) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// Window is a permitted interval on one weekday. It does not cross midnight.
type Window struct {
	Day   Day   `json:"day"`
	Start Clock `json:"start"`
	End   Clock `json:"end"`
}
