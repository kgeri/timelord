package enforcer

import (
	"fmt"
	"strings"
)

// User maps a local OS account to the account that Timetower knows. Local is
// used for session matching on the host; Server is used for schedule and
// status requests.
type User struct {
	Local  string
	Server string
}

// ParseUser parses a -user value. "name" maps name to itself; "local=server"
// maps a local OS account to a different Timetower account.
func ParseUser(value string) (User, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return User{}, fmt.Errorf("user must not be empty")
	}
	if strings.Count(value, "=") > 1 {
		return User{}, fmt.Errorf("user %q must use local=server", value)
	}

	local, server, ok := strings.Cut(value, "=")
	if !ok {
		return User{Local: value, Server: value}, nil
	}
	local = strings.TrimSpace(local)
	server = strings.TrimSpace(server)
	if local == "" || server == "" {
		return User{}, fmt.Errorf("user %q must use local=server", value)
	}
	return User{Local: local, Server: server}, nil
}

// String returns the mapping in -user syntax.
func (u User) String() string {
	if u.Local == u.Server {
		return u.Local
	}
	return u.Local + "=" + u.Server
}
