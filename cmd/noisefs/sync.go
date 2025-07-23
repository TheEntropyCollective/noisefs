package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/sync"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// handleSyncCommand handles all sync-related subcommands
func handleSyncCommand(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) == 0 {
		return showSyncUsage()
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "start":
		return handleSyncStart(subArgs, storageManager, quiet, jsonOutput)
	case "stop":
		return handleSyncStop(subArgs, storageManager, quiet, jsonOutput)
	case "status":
		return handleSyncStatus(subArgs, storageManager, quiet, jsonOutput)
	case "list":
		return handleSyncList(subArgs, storageManager, quiet, jsonOutput)
	case "pause":
		return handleSyncPause(subArgs, storageManager, quiet, jsonOutput)
	case "resume":
		return handleSyncResume(subArgs, storageManager, quiet, jsonOutput)
	default:
		return fmt.Errorf("unknown sync subcommand: %s", subcommand)
	}
}

// showSyncUsage shows usage information for sync commands
func showSyncUsage() error {
	fmt.Println("Usage: noisefs sync <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  start <sync-id> <local-path> <remote-path> [manifest-cid]  Start a new sync session")
	fmt.Println("  stop <sync-id>                                             Stop a sync session")
	fmt.Println("  status [sync-id]                                           Show sync status")
	fmt.Println("  list                                                       List all active syncs")
	fmt.Println("  pause <sync-id>                                            Pause a sync session")
	fmt.Println("  resume <sync-id>                                           Resume a sync session")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  noisefs sync start myproject /local/project /remote/project")
	fmt.Println("  noisefs sync status myproject")
	fmt.Println("  noisefs sync list")
	fmt.Println("  noisefs sync stop myproject")
	fmt.Println()
	return nil
}

// handleSyncStart starts a new sync session
func handleSyncStart(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: noisefs sync start <sync-id> <local-path> <remote-path> [manifest-cid]")
	}

	syncID := args[0]
	localPath := args[1]
	remotePath := args[2]
	manifestCID := ""

	if len(args) > 3 {
		manifestCID = args[3]
	}

	// Validate paths
	if !filepath.IsAbs(localPath) {
		return fmt.Errorf("local path must be absolute: %s", localPath)
	}

	// Check if local path exists
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return fmt.Errorf("local path does not exist: %s", localPath)
	}

	// Create sync engine
	syncEngine, err := createSyncEngine(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}
	defer syncEngine.Stop()

	// Start the sync
	startTime := time.Now()
	err = syncEngine.StartSync(syncID, localPath, remotePath, manifestCID)
	if err != nil {
		return fmt.Errorf("failed to start sync: %w", err)
	}

	// Get initial status
	session, err := syncEngine.GetSyncStatus(syncID)
	if err != nil {
		return fmt.Errorf("failed to get sync status: %w", err)
	}

	// Display results
	if jsonOutput {
		result := SyncStartResult{
			SyncID:     syncID,
			LocalPath:  localPath,
			RemotePath: remotePath,
			Status:     string(session.Status),
			StartTime:  startTime,
		}
		util.PrintJSONSuccess(result)
	} else if !quiet {
		fmt.Printf("Sync started successfully!\n")
		fmt.Printf("Sync ID: %s\n", syncID)
		fmt.Printf("Local Path: %s\n", localPath)
		fmt.Printf("Remote Path: %s\n", remotePath)
		fmt.Printf("Status: %s\n", session.Status)
		fmt.Printf("Started at: %s\n", startTime.Format(time.RFC3339))
	}

	return nil
}

// handleSyncStop stops a sync session
func handleSyncStop(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: noisefs sync stop <sync-id>")
	}

	syncID := args[0]

	// Create sync engine
	syncEngine, err := createSyncEngine(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}
	defer syncEngine.Stop()

	// Stop the sync
	err = syncEngine.StopSync(syncID)
	if err != nil {
		return fmt.Errorf("failed to stop sync: %w", err)
	}

	// Display results
	if jsonOutput {
		result := SyncStopResult{
			SyncID:   syncID,
			Stopped:  true,
			StopTime: time.Now(),
		}
		util.PrintJSONSuccess(result)
	} else if !quiet {
		fmt.Printf("Sync stopped successfully!\n")
		fmt.Printf("Sync ID: %s\n", syncID)
	}

	return nil
}

// handleSyncStatus shows the status of sync sessions
func handleSyncStatus(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	// Create sync engine
	syncEngine, err := createSyncEngine(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}
	defer syncEngine.Stop()

	if len(args) > 0 {
		// Show specific sync status
		syncID := args[0]
		session, err := syncEngine.GetSyncStatus(syncID)
		if err != nil {
			return fmt.Errorf("failed to get sync status: %w", err)
		}

		if jsonOutput {
			result := SyncStatusResult{
				SyncID:     session.SyncID,
				LocalPath:  session.LocalPath,
				RemotePath: session.RemotePath,
				Status:     string(session.Status),
				LastSync:   session.LastSync,
				Progress:   session.Progress,
			}
			util.PrintJSONSuccess(result)
		} else if quiet {
			fmt.Printf("%s\t%s\n", syncID, session.Status)
		} else {
			fmt.Printf("Sync Status: %s\n", syncID)
			fmt.Printf("Local Path: %s\n", session.LocalPath)
			fmt.Printf("Remote Path: %s\n", session.RemotePath)
			fmt.Printf("Status: %s\n", session.Status)
			fmt.Printf("Last Sync: %s\n", session.LastSync.Format(time.RFC3339))

			if session.Progress != nil {
				fmt.Printf("\nProgress:\n")
				
				// File progress
				if session.Progress.TotalFiles > 0 {
					fmt.Printf("  Files: %d/%d (%.1f%%)\n", 
						session.Progress.FilesProcessed, 
						session.Progress.TotalFiles,
						session.Progress.PercentComplete)
				}
				
				// Byte progress with human-readable format
				if session.Progress.TotalBytes > 0 {
					fmt.Printf("  Data: %s/%s\n", 
						formatBytes(session.Progress.BytesTransferred),
						formatBytes(session.Progress.TotalBytes))
				}
				
				// Throughput
				if session.Progress.CurrentThroughput > 0 {
					fmt.Printf("  Speed: %s/s\n", formatBytes(int64(session.Progress.CurrentThroughput)))
				}
				
				// Operations
				fmt.Printf("  Operations: %d completed, %d failed\n", 
					session.Progress.CompletedOperations, 
					session.Progress.FailedOperations)
				
				// Current operation
				if session.Progress.CurrentOperation != "" {
					fmt.Printf("  Current: %s\n", session.Progress.CurrentOperation)
				}
				
				// Time estimates
				if session.Progress.EstimatedCompletion > 0 {
					fmt.Printf("  ETA: %s\n", formatDuration(session.Progress.EstimatedCompletion))
				}
				
				// Elapsed time
				if !session.Progress.StartTime.IsZero() {
					elapsed := time.Since(session.Progress.StartTime)
					fmt.Printf("  Elapsed: %s\n", formatDuration(elapsed))
				}
			}
		}
	} else {
		// Show all active syncs
		sessions := syncEngine.ListActiveSyncs()

		if jsonOutput {
			results := make([]SyncStatusResult, len(sessions))
			for i, session := range sessions {
				results[i] = SyncStatusResult{
					SyncID:     session.SyncID,
					LocalPath:  session.LocalPath,
					RemotePath: session.RemotePath,
					Status:     string(session.Status),
					LastSync:   session.LastSync,
					Progress:   session.Progress,
				}
			}
			util.PrintJSONSuccess(results)
		} else if quiet {
			for _, session := range sessions {
				fmt.Printf("%s\t%s\n", session.SyncID, session.Status)
			}
		} else {
			if len(sessions) == 0 {
				fmt.Println("No active sync sessions")
			} else {
				fmt.Printf("Active Sync Sessions (%d):\n\n", len(sessions))
				for _, session := range sessions {
					fmt.Printf("ID: %s\n", session.SyncID)
					fmt.Printf("  Local: %s\n", session.LocalPath)
					fmt.Printf("  Remote: %s\n", session.RemotePath)
					fmt.Printf("  Status: %s\n", session.Status)
					fmt.Printf("  Last Sync: %s\n", session.LastSync.Format(time.RFC3339))
					fmt.Println()
				}
			}
		}
	}

	return nil
}

// handleSyncList lists all configured syncs
func handleSyncList(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	// Create sync engine
	syncEngine, err := createSyncEngine(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}
	defer syncEngine.Stop()

	// Get all active syncs
	sessions := syncEngine.ListActiveSyncs()

	// Get engine stats
	stats := syncEngine.GetStats()

	if jsonOutput {
		results := make([]SyncListResult, len(sessions))
		for i, session := range sessions {
			results[i] = SyncListResult{
				SyncID:     session.SyncID,
				LocalPath:  session.LocalPath,
				RemotePath: session.RemotePath,
				Status:     string(session.Status),
				LastSync:   session.LastSync,
			}
		}

		result := SyncListResponse{
			Sessions: results,
			Stats:    stats,
		}
		util.PrintJSONSuccess(result)
	} else if quiet {
		for _, session := range sessions {
			fmt.Printf("%s\t%s\t%s\t%s\n", session.SyncID, session.Status, session.LocalPath, session.RemotePath)
		}
	} else {
		fmt.Printf("NoiseFS Sync Status\n")
		fmt.Printf("Active Sessions: %d\n", stats.ActiveSessions)
		fmt.Printf("Total Events: %d\n", stats.TotalSyncEvents)
		fmt.Printf("Total Conflicts: %d\n", stats.TotalConflicts)
		fmt.Printf("Total Errors: %d\n", stats.TotalErrors)
		if !stats.LastSyncTime.IsZero() {
			fmt.Printf("Last Sync: %s\n", stats.LastSyncTime.Format(time.RFC3339))
		}
		fmt.Println()

		if len(sessions) == 0 {
			fmt.Println("No active sync sessions")
		} else {
			fmt.Printf("Active Sync Sessions:\n\n")
			for _, session := range sessions {
				fmt.Printf("%-15s %-10s %s\n", session.SyncID, session.Status, session.LocalPath)
				fmt.Printf("%-15s %-10s %s\n", "", "", session.RemotePath)
				fmt.Printf("%-15s %-10s %s\n", "", "", session.LastSync.Format("2006-01-02 15:04:05"))
				fmt.Println()
			}
		}
	}

	return nil
}

// handleSyncPause pauses a sync session
func handleSyncPause(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: noisefs sync pause <sync-id>")
	}

	syncID := args[0]

	// Create sync engine
	syncEngine, err := createSyncEngine(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}
	defer syncEngine.Stop()

	// Pause the sync
	err = syncEngine.PauseSync(syncID)
	if err != nil {
		return fmt.Errorf("failed to pause sync: %w", err)
	}

	// Display results
	if jsonOutput {
		result := SyncActionResult{
			SyncID:    syncID,
			Action:    "pause",
			Success:   true,
			Timestamp: time.Now(),
		}
		util.PrintJSONSuccess(result)
	} else if !quiet {
		fmt.Printf("Sync paused successfully!\n")
		fmt.Printf("Sync ID: %s\n", syncID)
	}

	return nil
}

// handleSyncResume resumes a sync session
func handleSyncResume(args []string, storageManager *storage.Manager, quiet bool, jsonOutput bool) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: noisefs sync resume <sync-id>")
	}

	syncID := args[0]

	// Create sync engine
	syncEngine, err := createSyncEngine(storageManager)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}
	defer syncEngine.Stop()

	// Resume the sync
	err = syncEngine.ResumeSync(syncID)
	if err != nil {
		return fmt.Errorf("failed to resume sync: %w", err)
	}

	// Display results
	if jsonOutput {
		result := SyncActionResult{
			SyncID:    syncID,
			Action:    "resume",
			Success:   true,
			Timestamp: time.Now(),
		}
		util.PrintJSONSuccess(result)
	} else if !quiet {
		fmt.Printf("Sync resumed successfully!\n")
		fmt.Printf("Sync ID: %s\n", syncID)
	}

	return nil
}

// createSyncEngine creates a configured sync engine
func createSyncEngine(storageManager *storage.Manager) (*sync.SyncEngine, error) {
	// Get user config directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Create sync state directory
	syncStateDir := filepath.Join(homeDir, ".noisefs", "sync")
	if err := os.MkdirAll(syncStateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sync state directory: %w", err)
	}

	// Create sync state store
	stateStore, err := sync.NewSyncStateStore(syncStateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync state store: %w", err)
	}

	// Create sync configuration
	syncConfig := &sync.SyncConfig{
		SyncInterval:       time.Minute,
		ConflictResolution: sync.ConflictResolvePrompt,
		MaxRetries:         3,
		WatchMode:          true,
	}

	// Create file watcher
	fileWatcher, err := sync.NewFileWatcher(syncConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Create directory manager
	encryptionKey, err := crypto.GenerateKey("sync-key")
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	directoryManager, err := storage.NewDirectoryManager(storageManager, encryptionKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory manager: %w", err)
	}

	// Create remote change monitor
	remoteMonitor, err := sync.NewRemoteChangeMonitor(directoryManager, stateStore, syncConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote change monitor: %w", err)
	}

	// Create sync engine
	syncEngine, err := sync.NewSyncEngine(stateStore, fileWatcher, remoteMonitor, directoryManager, syncConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync engine: %w", err)
	}

	return syncEngine, nil
}

// Result structs for JSON output
type SyncStartResult struct {
	SyncID     string    `json:"sync_id"`
	LocalPath  string    `json:"local_path"`
	RemotePath string    `json:"remote_path"`
	Status     string    `json:"status"`
	StartTime  time.Time `json:"start_time"`
}

type SyncStopResult struct {
	SyncID   string    `json:"sync_id"`
	Stopped  bool      `json:"stopped"`
	StopTime time.Time `json:"stop_time"`
}

type SyncStatusResult struct {
	SyncID     string             `json:"sync_id"`
	LocalPath  string             `json:"local_path"`
	RemotePath string             `json:"remote_path"`
	Status     string             `json:"status"`
	LastSync   time.Time          `json:"last_sync"`
	Progress   *sync.SyncProgress `json:"progress,omitempty"`
}

type SyncListResult struct {
	SyncID     string    `json:"sync_id"`
	LocalPath  string    `json:"local_path"`
	RemotePath string    `json:"remote_path"`
	Status     string    `json:"status"`
	LastSync   time.Time `json:"last_sync"`
}

type SyncListResponse struct {
	Sessions []SyncListResult      `json:"sessions"`
	Stats    *sync.SyncEngineStats `json:"stats"`
}

type SyncActionResult struct {
	SyncID    string    `json:"sync_id"`
	Action    string    `json:"action"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

