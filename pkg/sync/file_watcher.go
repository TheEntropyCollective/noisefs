package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches local directory for changes and emits sync events
type FileWatcher struct {
	watcher       *fsnotify.Watcher
	watchedPaths  map[string]bool
	eventChan     chan SyncEvent
	errorChan     chan error
	config        *SyncConfig
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	debounceTimer map[string]*time.Timer
	debounceMu    sync.Mutex
}

// NewFileWatcher creates a new file watcher with the given configuration
func NewFileWatcher(config *SyncConfig) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		watcher:       watcher,
		watchedPaths:  make(map[string]bool),
		eventChan:     make(chan SyncEvent, 100),
		errorChan:     make(chan error, 10),
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		debounceTimer: make(map[string]*time.Timer),
	}

	go fw.eventLoop()

	return fw, nil
}

// AddPath adds a directory path to be watched for changes
func (fw *FileWatcher) AddPath(path string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.watchedPaths[path] {
		return nil // Already watching this path
	}

	// Check if path exists
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}

	// Add the path to fsnotify watcher
	if err := fw.watcher.Add(path); err != nil {
		return fmt.Errorf("failed to add path to watcher: %w", err)
	}

	fw.watchedPaths[path] = true

	// If watching recursively, add all subdirectories
	if fw.config.WatchMode {
		err := filepath.Walk(path, func(subPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() && subPath != path {
				if !fw.shouldIgnorePath(subPath) {
					if err := fw.watcher.Add(subPath); err != nil {
						return fmt.Errorf("failed to add subdirectory to watcher: %w", err)
					}
					fw.watchedPaths[subPath] = true
				}
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to add subdirectories to watcher: %w", err)
		}
	}

	return nil
}

// RemovePath removes a directory path from being watched
func (fw *FileWatcher) RemovePath(path string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if !fw.watchedPaths[path] {
		return nil // Not watching this path
	}

	if err := fw.watcher.Remove(path); err != nil {
		return fmt.Errorf("failed to remove path from watcher: %w", err)
	}

	delete(fw.watchedPaths, path)

	// Remove any subdirectories as well
	for watchedPath := range fw.watchedPaths {
		if strings.HasPrefix(watchedPath, path+string(os.PathSeparator)) {
			if err := fw.watcher.Remove(watchedPath); err != nil {
				// Log error but don't fail the operation
				select {
				case fw.errorChan <- fmt.Errorf("failed to remove subdirectory %s: %w", watchedPath, err):
				default:
				}
			}
			delete(fw.watchedPaths, watchedPath)
		}
	}

	return nil
}

// Events returns a channel that receives sync events
func (fw *FileWatcher) Events() <-chan SyncEvent {
	return fw.eventChan
}

// Errors returns a channel that receives errors
func (fw *FileWatcher) Errors() <-chan error {
	return fw.errorChan
}

// Stop stops the file watcher and closes all channels
func (fw *FileWatcher) Stop() error {
	fw.cancel()

	fw.mu.Lock()
	defer fw.mu.Unlock()

	if err := fw.watcher.Close(); err != nil {
		return fmt.Errorf("failed to close watcher: %w", err)
	}

	close(fw.eventChan)
	close(fw.errorChan)

	return nil
}

// eventLoop processes fsnotify events and converts them to sync events
func (fw *FileWatcher) eventLoop() {
	defer func() {
		// Clean up any pending debounce timers
		fw.debounceMu.Lock()
		for _, timer := range fw.debounceTimer {
			timer.Stop()
		}
		fw.debounceMu.Unlock()
	}()

	for {
		select {
		case <-fw.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			fw.handleFsEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}

			select {
			case fw.errorChan <- err:
			default:
			}
		}
	}
}

// handleFsEvent processes a single fsnotify event
func (fw *FileWatcher) handleFsEvent(event fsnotify.Event) {
	// Check if we should ignore this file
	if fw.shouldIgnorePath(event.Name) {
		return
	}

	// Debounce rapid events on the same file
	fw.debounceMu.Lock()
	if timer, exists := fw.debounceTimer[event.Name]; exists {
		timer.Stop()
	}

	fw.debounceTimer[event.Name] = time.AfterFunc(100*time.Millisecond, func() {
		fw.processEvent(event)

		fw.debounceMu.Lock()
		delete(fw.debounceTimer, event.Name)
		fw.debounceMu.Unlock()
	})
	fw.debounceMu.Unlock()
}

// processEvent converts fsnotify event to sync event
func (fw *FileWatcher) processEvent(event fsnotify.Event) {
	var syncEvent SyncEvent

	// Get file info if file still exists
	var fileInfo os.FileInfo
	var err error
	if event.Has(fsnotify.Remove) {
		// File was removed, we can't get info
		syncEvent = SyncEvent{
			Type:      fw.getEventType(event, false),
			Path:      event.Name,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"fs_event": event.Op.String(),
			},
		}
	} else {
		fileInfo, err = os.Stat(event.Name)
		if err != nil {
			// File might have been removed between event and stat
			syncEvent = SyncEvent{
				Type:      EventTypeFileDeleted,
				Path:      event.Name,
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"fs_event": event.Op.String(),
					"error":    err.Error(),
				},
			}
		} else {
			syncEvent = SyncEvent{
				Type:      fw.getEventType(event, fileInfo.IsDir()),
				Path:      event.Name,
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"fs_event": event.Op.String(),
					"size":     fileInfo.Size(),
					"mode":     fileInfo.Mode(),
					"mod_time": fileInfo.ModTime(),
					"is_dir":   fileInfo.IsDir(),
				},
			}

			// If a new directory was created, add it to watcher
			if event.Has(fsnotify.Create) && fileInfo.IsDir() && fw.config.WatchMode {
				fw.mu.Lock()
				if !fw.watchedPaths[event.Name] {
					if err := fw.watcher.Add(event.Name); err != nil {
						select {
						case fw.errorChan <- fmt.Errorf("failed to add new directory to watcher: %w", err):
						default:
						}
					} else {
						fw.watchedPaths[event.Name] = true
					}
				}
				fw.mu.Unlock()
			}
		}
	}

	// Send the event
	select {
	case fw.eventChan <- syncEvent:
	case <-fw.ctx.Done():
		return
	default:
		// Channel is full, log error but don't block
		select {
		case fw.errorChan <- fmt.Errorf("event channel full, dropping event for %s", event.Name):
		default:
		}
	}
}

// getEventType converts fsnotify event to sync event type
func (fw *FileWatcher) getEventType(event fsnotify.Event, isDir bool) EventType {
	if isDir {
		if event.Has(fsnotify.Create) {
			return EventTypeDirCreated
		}
		if event.Has(fsnotify.Remove) {
			return EventTypeDirDeleted
		}
		return EventTypeDirCreated // Default for directory events
	}

	if event.Has(fsnotify.Create) {
		return EventTypeFileCreated
	}
	if event.Has(fsnotify.Write) {
		return EventTypeFileModified
	}
	if event.Has(fsnotify.Remove) {
		return EventTypeFileDeleted
	}
	if event.Has(fsnotify.Rename) {
		return EventTypeFileDeleted // Treat rename as delete + create
	}

	return EventTypeFileModified // Default for file events
}

// shouldIgnorePath checks if a path should be ignored based on patterns
func (fw *FileWatcher) shouldIgnorePath(path string) bool {
	filename := filepath.Base(path)

	// Check exclude patterns first
	for _, pattern := range fw.config.ExcludePatterns {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
	}

	// If include patterns are specified, check them
	if len(fw.config.IncludePatterns) > 0 {
		for _, pattern := range fw.config.IncludePatterns {
			if matched, _ := filepath.Match(pattern, filename); matched {
				return false
			}
		}
		return true // Not in include patterns, so ignore
	}

	return false
}

// GetWatchedPaths returns a copy of currently watched paths
func (fw *FileWatcher) GetWatchedPaths() []string {
	fw.mu.RLock()
	defer fw.mu.RUnlock()

	paths := make([]string, 0, len(fw.watchedPaths))
	for path := range fw.watchedPaths {
		paths = append(paths, path)
	}

	return paths
}
