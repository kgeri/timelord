package timetower

import (
	"os"
	"path/filepath"
	"testing"
)

func testdataPath(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

func readTestdata(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(testdataPath(parts...))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return string(data)
}
