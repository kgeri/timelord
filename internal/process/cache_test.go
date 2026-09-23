package process

import "testing"

func TestCacheGroupsByUserAndExecutable(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1, User: "alice", Name: "bash", Executable: "/bin/bash", MemoryBytes: 100},
		{PID: 2, User: "alice", Name: "bash", Executable: "/bin/bash", MemoryBytes: 200},
		{PID: 3, User: "bob", Name: "bash", Executable: "/bin/bash", MemoryBytes: 300},
		{PID: 4, User: "alice", Name: "chrome", Executable: "/opt/chrome", MemoryBytes: 400},
	})

	aliceBash, ok := cache.Get("alice", "/bin/bash")
	if !ok {
		t.Fatal("expected alice /bin/bash in cache")
	}
	if aliceBash.Count != 2 {
		t.Errorf("alice bash count = %d, want 2", aliceBash.Count)
	}
	if aliceBash.MemoryBytes != 300 {
		t.Errorf("alice bash memory = %d, want 300", aliceBash.MemoryBytes)
	}

	bobBash, ok := cache.Get("bob", "/bin/bash")
	if !ok {
		t.Fatal("expected bob /bin/bash in cache")
	}
	if bobBash.Count != 1 {
		t.Errorf("bob bash count = %d, want 1", bobBash.Count)
	}

	if _, ok := cache.Get("bob", "/opt/chrome"); ok {
		t.Error("bob should not have /opt/chrome")
	}
}

func TestCacheGetMissing(t *testing.T) {
	cache := NewCache()
	if _, ok := cache.Get("alice", "/does/not/exist"); ok {
		t.Error("expected a miss for an unknown executable")
	}
}

func TestCacheSnapshotSorted(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{User: "bob", Executable: "/bin/bash"},
		{User: "alice", Executable: "/z"},
		{User: "alice", Executable: "/a"},
	})

	got := cache.Snapshot()
	want := []key{
		{user: "alice", executable: "/a"},
		{user: "alice", executable: "/z"},
		{user: "bob", executable: "/bin/bash"},
	}
	if len(got) != len(want) {
		t.Fatalf("snapshot length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].User != want[i].user || got[i].Executable != want[i].executable {
			t.Errorf("snapshot[%d] = (%q, %q), want (%q, %q)",
				i, got[i].User, got[i].Executable, want[i].user, want[i].executable)
		}
	}
}

func TestCacheSnapshotIsCopy(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{{User: "alice", Executable: "/bin/bash"}})

	snapshot := cache.Snapshot()
	snapshot[0].Executable = "/changed"

	entry, _ := cache.Get("alice", "/bin/bash")
	if entry.Executable != "/bin/bash" {
		t.Errorf("snapshot change affected the cache: got %q", entry.Executable)
	}
}

func TestCacheAccumulatesCPU(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 10},
	})

	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 0 {
		t.Errorf("CPU after first sample = %v, want 0", e.CPUSeconds)
	}

	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 12.5},
	})
	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 2.5 {
		t.Errorf("CPU after second sample = %v, want 2.5", e.CPUSeconds)
	}

	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 13},
	})
	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 3 {
		t.Errorf("CPU after third sample = %v, want 3", e.CPUSeconds)
	}
}

func TestCacheCPUHandlesRestart(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 10},
	})

	// The process exits and a new process gets the same PID.
	cache.Set([]Process{
		{PID: 1001, StartTime: 2, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 100},
	})
	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 0 {
		t.Errorf("CPU after restart = %v, want 0", e.CPUSeconds)
	}

	cache.Set([]Process{
		{PID: 1001, StartTime: 2, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 101},
	})
	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 1 {
		t.Errorf("CPU after restart sample = %v, want 1", e.CPUSeconds)
	}
}

func TestCacheResetsCPUWhenEntryDisappears(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 10},
	})
	cache.Set([]Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 11},
	})
	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 1 {
		t.Fatalf("CPU = %v, want 1", e.CPUSeconds)
	}

	// All processes of this entry exit.
	cache.Set(nil)
	if _, ok := cache.Get("alice", "/bin/bash"); ok {
		t.Fatal("entry should be gone")
	}

	// The executable runs again later.
	cache.Set([]Process{
		{PID: 1002, StartTime: 2, User: "alice", Name: "bash", Executable: "/bin/bash", CPUSeconds: 50},
	})
	if e, _ := cache.Get("alice", "/bin/bash"); e.CPUSeconds != 0 {
		t.Errorf("CPU after the executable returns = %v, want 0", e.CPUSeconds)
	}
}
