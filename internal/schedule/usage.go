package schedule

import "time"

// Usage is the active time counted for one managed user in the current local
// day. Limit is zero when the policy has no daily limit; HasPolicy is false
// when the user has no schedule at all, which means no time is allowed.
type Usage struct {
	User      string
	Used      time.Duration
	Limit     time.Duration
	HasPolicy bool
}
