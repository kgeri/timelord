package schedule

import (
	"sync"
	"time"
)

// Store is a concurrency-safe view of the current schedule for managed users.
// Policies for users that were not configured with -user are ignored.
type Store struct {
	mu       sync.RWMutex
	users    []string
	policies map[string]Policy
}

// NewStore returns a store for the given managed users.
func NewStore(users []string) *Store {
	unique := make([]string, 0, len(users))
	seen := make(map[string]bool, len(users))
	for _, user := range users {
		if seen[user] {
			continue
		}
		seen[user] = true
		unique = append(unique, user)
	}

	return &Store{
		users:    unique,
		policies: make(map[string]Policy),
	}
}

// Replace replaces the current policies. A managed user without a policy in
// config is denied.
func (s *Store) Replace(config *Config) {
	policies := make(map[string]Policy)
	if config != nil {
		for _, policy := range config.Users {
			policies[policy.User] = policy
		}
	}

	managed := make(map[string]Policy, len(s.users))
	for _, user := range s.users {
		if policy, ok := policies[user]; ok {
			managed[user] = policy
		}
	}

	s.mu.Lock()
	s.policies = managed
	s.mu.Unlock()
}

// Users returns the managed user names.
func (s *Store) Users() []string {
	out := make([]string, len(s.users))
	copy(out, s.users)
	return out
}

// Policy returns the policy for user.
func (s *Store) Policy(user string) (Policy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	policy, ok := s.policies[user]
	return policy, ok
}

// Allowed reports whether user may use the computer at now after used active
// time. A user without a policy is never allowed.
func (s *Store) Allowed(user string, now time.Time, used time.Duration) bool {
	policy, ok := s.Policy(user)
	if !ok {
		return false
	}
	return policy.Allowed(now, used)
}
