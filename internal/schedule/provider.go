package schedule

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// File maps a managed user to the schedule file in that user's home directory.
type File struct {
	User string
	Path string
}

// Provider supplies the current schedule. Load returns the complete schedule
// for all managed users. Watch reports later changes until ctx is cancelled.
type Provider interface {
	Load() (*Config, error)
	Watch(ctx context.Context, update func(*Config, error))
}

// FileProvider loads one schedule file per managed user and reloads all files
// when one of them changes.
type FileProvider struct {
	files []File

	mu     sync.Mutex
	warned map[string]bool
}

// NewFileProvider returns a provider for files.
func NewFileProvider(files []File) *FileProvider {
	return &FileProvider{
		files:  append([]File(nil), files...),
		warned: make(map[string]bool),
	}
}

// Load reads every schedule file. A missing file is an empty policy for that
// user. A file that exists must be valid.
func (p *FileProvider) Load() (*Config, error) {
	config := &Config{}

	for _, file := range p.files {
		fileConfig, err := Load(file.Path)
		if errors.Is(err, fs.ErrNotExist) {
			p.warnOnce("missing:"+file.Path,
				"schedule %s for user %s is missing: no time allowed", file.Path, file.User)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.Path, err)
		}
		p.clearWarn("missing:" + file.Path)

		policy, ok := fileConfig.ForUser(file.User)
		if !ok {
			p.warnOnce("policy:"+file.Path,
				"schedule %s has no policy for user %s: no time allowed", file.Path, file.User)
			continue
		}
		p.clearWarn("policy:" + file.Path)
		config.Users = append(config.Users, policy)
	}
	return config, nil
}

// Watch reloads the schedule whenever one of the schedule files changes.
func (p *FileProvider) Watch(ctx context.Context, update func(*Config, error)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		update(nil, fmt.Errorf("create schedule watcher: %w", err))
		return
	}
	defer watcher.Close()

	dirs := make(map[string]bool)
	for _, file := range p.files {
		dir := filepath.Dir(file.Path)
		if dirs[dir] {
			continue
		}
		dirs[dir] = true
		if err := watcher.Add(dir); err != nil {
			update(nil, fmt.Errorf("watch schedule directory %s: %w", dir, err))
		}
	}

	var reload <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !p.matches(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}
			// Coalesce the writes from a single upload and give the writer a
			// moment to finish the file.
			reload = time.After(100 * time.Millisecond)
		case <-reload:
			reload = nil
			config, err := p.Load()
			update(config, err)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			update(nil, fmt.Errorf("schedule watcher: %w", err))
		}
	}
}

// matches reports whether name is one of the watched schedule files.
func (p *FileProvider) matches(name string) bool {
	name = filepath.Clean(name)
	for _, file := range p.files {
		if filepath.Clean(file.Path) == name {
			return true
		}
	}
	return false
}

func (p *FileProvider) warnOnce(key, format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.warned[key] {
		return
	}
	p.warned[key] = true
	log.Printf("WARN: "+format, args...)
}

func (p *FileProvider) clearWarn(key string) {
	p.mu.Lock()
	delete(p.warned, key)
	p.mu.Unlock()
}
