package enforcer

import (
	"os"
	"path/filepath"
	"testing"
)

func readTestdata(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return data
}
