package process

import "strings"

// windowsScope classifies a Windows process as an app or a system process.
// A process is a system process when it runs in session 0, when its owner is a
// system or service account, or when its executable matches one of the system
// path prefixes. The Windows lister builds those prefixes from the system
// directories and the Windows inbox app packages.
//
// The mapping is platform-independent so it can be unit tested everywhere.
func windowsScope(sessionID uint32, executable string, systemOwner bool, systemPaths []string) Scope {
	if sessionID == 0 || systemOwner || hasAnyPrefix(executable, systemPaths) {
		return ScopeSystem
	}
	return ScopeApp
}

// hasAnyPrefix reports whether path starts with one of the prefixes. The
// comparison is case-insensitive. Directory prefixes include their trailing
// separator; package prefixes do not.
func hasAnyPrefix(path string, prefixes []string) bool {
	if path == "" {
		return false
	}

	path = strings.ToLower(path)
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(path, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}
