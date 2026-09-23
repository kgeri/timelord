package scheduler

import (
	"errors"
	"testing"

	"timelord/internal/process"
)

type fakeLister struct {
	processes []process.Process
	err       error
}

func (f fakeLister) List() ([]process.Process, error) {
	return f.processes, f.err
}

func TestUpdateFillsCache(t *testing.T) {
	cache := process.NewCache()
	s := New(fakeLister{processes: []process.Process{
		{Name: "bash", Executable: "/bin/bash"},
		{Name: "bash", Executable: "/bin/bash"},
	}}, cache)

	s.update()

	entry, ok := cache.Get("/bin/bash")
	if !ok {
		t.Fatal("expected /bin/bash in cache")
	}
	if entry.Count != 2 {
		t.Errorf("count = %d, want 2", entry.Count)
	}
}

func TestUpdateKeepsCacheOnError(t *testing.T) {
	cache := process.NewCache()
	cache.Set([]process.Process{{Executable: "/bin/bash"}})

	s := New(fakeLister{err: errors.New("list failed")}, cache)
	s.update()

	if _, ok := cache.Get("/bin/bash"); !ok {
		t.Error("cache lost its entries after a list error")
	}
}
