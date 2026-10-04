package process

import "testing"

func TestCacheGroupsByUserAndExecutable(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, MemoryBytes: 100},
		{PID: 2, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, MemoryBytes: 200},
		{PID: 3, User: "bob", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, MemoryBytes: 300},
		{PID: 4, User: "alice", Name: "chrome", Executable: "/opt/chrome", Scope: ScopeApp, MemoryBytes: 400},
	})

	aliceBash, ok := cache.Get("alice", "/bin/bash", ScopeApp)
	if !ok {
		t.Fatal("expected alice /bin/bash in cache")
	}
	if aliceBash.Count != 2 {
		t.Errorf("alice bash count = %d, want 2", aliceBash.Count)
	}
	if aliceBash.MemoryBytes != 300 {
		t.Errorf("alice bash memory = %d, want 300", aliceBash.MemoryBytes)
	}

	bobBash, ok := cache.Get("bob", "/bin/bash", ScopeApp)
	if !ok {
		t.Fatal("expected bob /bin/bash in cache")
	}
	if bobBash.Count != 1 {
		t.Errorf("bob bash count = %d, want 1", bobBash.Count)
	}

	if _, ok := cache.Get("bob", "/opt/chrome", ScopeApp); ok {
		t.Error("bob should not have /opt/chrome")
	}
}

func TestCacheSeparatesScopes(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp},
		{PID: 2, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeSystem},
	})

	app, ok := cache.Get("alice", "/bin/bash", ScopeApp)
	if !ok {
		t.Fatal("expected alice /bin/bash in app scope")
	}
	if app.Count != 1 || app.Scope != ScopeApp {
		t.Errorf("app entry = %+v, want one process in scope %q", app, ScopeApp)
	}

	system, ok := cache.Get("alice", "/bin/bash", ScopeSystem)
	if !ok {
		t.Fatal("expected alice /bin/bash in system scope")
	}
	if system.Count != 1 || system.Scope != ScopeSystem {
		t.Errorf("system entry = %+v, want one process in scope %q", system, ScopeSystem)
	}
}

func TestCacheGetMissing(t *testing.T) {
	cache := NewCache()
	if _, ok := cache.Get("alice", "/does/not/exist", ScopeApp); ok {
		t.Error("expected a miss for an unknown executable")
	}
}

func TestCacheSnapshotSorted(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{User: "bob", Executable: "/bin/bash", Scope: ScopeApp},
		{User: "alice", Executable: "/z", Scope: ScopeApp},
		{User: "alice", Executable: "/a", Scope: ScopeSystem},
		{User: "alice", Executable: "/a", Scope: ScopeApp},
	})

	got := cache.Snapshot()
	want := []key{
		{user: "alice", executable: "/a", scope: ScopeApp},
		{user: "alice", executable: "/a", scope: ScopeSystem},
		{user: "alice", executable: "/z", scope: ScopeApp},
		{user: "bob", executable: "/bin/bash", scope: ScopeApp},
	}
	if len(got) != len(want) {
		t.Fatalf("snapshot length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].User != want[i].user || got[i].Executable != want[i].executable || got[i].Scope != want[i].scope {
			t.Errorf("snapshot[%d] = (%q, %q, %q), want (%q, %q, %q)",
				i, got[i].User, got[i].Executable, got[i].Scope, want[i].user, want[i].executable, want[i].scope)
		}
	}
}

func TestCacheSnapshotIsCopy(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{{User: "alice", Executable: "/bin/bash", Scope: ScopeApp}})

	snapshot := cache.Snapshot()
	snapshot[0].Executable = "/changed"

	entry, _ := cache.Get("alice", "/bin/bash", ScopeApp)
	if entry.Executable != "/bin/bash" {
		t.Errorf("snapshot change affected the cache: got %q", entry.Executable)
	}
}

func TestCacheAccumulatesCPU(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 10},
	})

	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 0 {
		t.Errorf("CPU after first sample = %v, want 0", e.CPUSeconds)
	}

	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 12.5},
	})
	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 2.5 {
		t.Errorf("CPU after second sample = %v, want 2.5", e.CPUSeconds)
	}

	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 13},
	})
	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 3 {
		t.Errorf("CPU after third sample = %v, want 3", e.CPUSeconds)
	}
}

func TestCacheCPUHandlesRestart(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 10},
	})

	// The process exits and a new process gets the same PID.
	cache.Set([]Process{
		{PID: 1001, StartTime: 2, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 100},
	})
	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 0 {
		t.Errorf("CPU after restart = %v, want 0", e.CPUSeconds)
	}

	cache.Set([]Process{
		{PID: 1001, StartTime: 2, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 101},
	})
	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 1 {
		t.Errorf("CPU after restart sample = %v, want 1", e.CPUSeconds)
	}
}

func TestCacheResetsCPUWhenEntryDisappears(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 10},
	})
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 11},
	})
	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 1 {
		t.Fatalf("CPU = %v, want 1", e.CPUSeconds)
	}

	// All processes of this entry exit.
	cache.Set(nil)
	if _, ok := cache.Get("alice", "/bin/bash", ScopeApp); ok {
		t.Fatal("entry should be gone")
	}

	// The executable runs again later.
	cache.Set([]Process{
		{PID: 1002, StartTime: 2, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: ScopeApp, CPUSeconds: 50},
	})
	if e, _ := cache.Get("alice", "/bin/bash", ScopeApp); e.CPUSeconds != 0 {
		t.Errorf("CPU after the executable returns = %v, want 0", e.CPUSeconds)
	}
}
