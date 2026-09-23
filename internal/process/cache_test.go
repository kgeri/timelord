package process

import "testing"

func TestCacheSetCountsByExecutable(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{User: "alice", Name: "bash", Executable: "/bin/bash"},
		{User: "alice", Name: "bash", Executable: "/bin/bash"},
		{User: "alice", Name: "chrome", Executable: "/opt/chrome"},
	})

	bash, ok := cache.Get("/bin/bash")
	if !ok {
		t.Fatal("expected /bin/bash in cache")
	}
	if bash.Count != 2 {
		t.Errorf("bash count = %d, want 2", bash.Count)
	}
	if bash.Name != "bash" {
		t.Errorf("bash name = %q, want %q", bash.Name, "bash")
	}
	if bash.User != "alice" {
		t.Errorf("bash user = %q, want %q", bash.User, "alice")
	}

	chrome, ok := cache.Get("/opt/chrome")
	if !ok {
		t.Fatal("expected /opt/chrome in cache")
	}
	if chrome.Count != 1 {
		t.Errorf("chrome count = %d, want 1", chrome.Count)
	}
}

func TestCacheGetMissing(t *testing.T) {
	cache := NewCache()
	if _, ok := cache.Get("/does/not/exist"); ok {
		t.Error("expected a miss for an unknown executable")
	}
}

func TestCacheSnapshotSortedByExecutable(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{
		{Executable: "/z"},
		{Executable: "/a"},
		{Executable: "/m"},
	})

	got := cache.Snapshot()
	want := []string{"/a", "/m", "/z"}
	if len(got) != len(want) {
		t.Fatalf("snapshot length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Executable != want[i] {
			t.Errorf("snapshot[%d] = %q, want %q", i, got[i].Executable, want[i])
		}
	}
}

func TestCacheSnapshotIsCopy(t *testing.T) {
	cache := NewCache()
	cache.Set([]Process{{Executable: "/bin/bash"}})

	snapshot := cache.Snapshot()
	snapshot[0].Executable = "/changed"

	entry, _ := cache.Get("/bin/bash")
	if entry.Executable != "/bin/bash" {
		t.Errorf("snapshot change affected the cache: got %q", entry.Executable)
	}
}
