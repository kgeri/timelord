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
		{User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeApp},
		{User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeApp},
	}}, cache)

	s.update()

	entry, ok := cache.Get("alice", "/bin/bash", process.ScopeApp)
	if !ok {
		t.Fatal("expected alice /bin/bash in cache")
	}
	if entry.Count != 2 {
		t.Errorf("count = %d, want 2", entry.Count)
	}
}

func TestUpdateKeepsCacheOnError(t *testing.T) {
	cache := process.NewCache()
	cache.Set([]process.Process{{User: "alice", Executable: "/bin/bash", Scope: process.ScopeApp}})

	s := New(fakeLister{err: errors.New("list failed")}, cache)
	s.update()

	if _, ok := cache.Get("alice", "/bin/bash", process.ScopeApp); !ok {
		t.Error("cache lost its entries after a list error")
	}
}
