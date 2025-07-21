package sync

import (
	"time"
)

// EventType represents the type of sync event
type EventType string

const (
	EventTypeFileCreated  EventType = "file_created"
	EventTypeFileModified EventType = "file_modified"
	EventTypeFileDeleted  EventType = "file_deleted"
	EventTypeDirCreated   EventType = "dir_created"
	EventTypeDirDeleted   EventType = "dir_deleted"
)

// SyncEvent represents a change event in the sync system
type SyncEvent struct {
	Type      EventType              `json:"type"`
	Path      string                 `json:"path"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// FileMetadata represents metadata for a local file
type FileMetadata struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	IsDir       bool      `json:"is_dir"`
	Checksum    string    `json:"checksum,omitempty"`
	Permissions uint32    `json:"permissions"`
	// Additional fields for move detection
	Inode  uint64 `json:"inode,omitempty"`
	Device uint64 `json:"device,omitempty"`
}

// RemoteMetadata represents metadata for a remote file in NoiseFS
type RemoteMetadata struct {
	Path          string    `json:"path"`
	DescriptorCID string    `json:"descriptor_cid"`
	ContentCID    string    `json:"content_cid,omitempty"`
	Size          int64     `json:"size"`
	ModTime       time.Time `json:"mod_time"`
	IsDir         bool      `json:"is_dir"`
	EncryptionKey string    `json:"encryption_key,omitempty"`
	// Additional tracking for sync accuracy
	LastSyncTime time.Time `json:"last_sync_time"`
	Version      int64     `json:"version"`
}

// SyncOperation represents a sync operation to be performed
type SyncOperation struct {
	ID         string        `json:"id"`
	Type       OperationType `json:"type"`
	LocalPath  string        `json:"local_path"`
	RemotePath string        `json:"remote_path"`
	Timestamp  time.Time     `json:"timestamp"`
	Status     OpStatus      `json:"status"`
	Retries    int           `json:"retries"`
	Error      string        `json:"error,omitempty"`
}

// OperationType represents the type of sync operation
type OperationType string

const (
	OpTypeUpload    OperationType = "upload"
	OpTypeDownload  OperationType = "download"
	OpTypeDelete    OperationType = "delete"
	OpTypeCreateDir OperationType = "create_dir"
	OpTypeDeleteDir OperationType = "delete_dir"
	OpTypeMove      OperationType = "move"
	OpTypeRename    OperationType = "rename"
)

// OpStatus represents the status of a sync operation
type OpStatus string

const (
	OpStatusPending   OpStatus = "pending"
	OpStatusRunning   OpStatus = "running"
	OpStatusCompleted OpStatus = "completed"
	OpStatusFailed    OpStatus = "failed"
	OpStatusConflict  OpStatus = "conflict"
)

// SyncState represents the current state of synchronization
type SyncState struct {
	LocalPath      string                    `json:"local_path"`
	RemotePath     string                    `json:"remote_path"`
	LocalSnapshot  map[string]FileMetadata   `json:"local_snapshot"`
	RemoteSnapshot map[string]RemoteMetadata `json:"remote_snapshot"`
	SyncHistory    []SyncOperation           `json:"sync_history"`
	PendingOps     []SyncOperation           `json:"pending_ops"`
	LastSync       time.Time                 `json:"last_sync"`
	SyncEnabled    bool                      `json:"sync_enabled"`
}

// ConflictResolution represents how conflicts should be resolved
type ConflictResolution string

const (
	ConflictResolveLocal     ConflictResolution = "local"
	ConflictResolveRemote    ConflictResolution = "remote"
	ConflictResolveTimestamp ConflictResolution = "timestamp"
	ConflictResolvePrompt    ConflictResolution = "prompt"
)

// Conflict represents a sync conflict that needs resolution
type Conflict struct {
	ID             string             `json:"id"`
	LocalPath      string             `json:"local_path"`
	RemotePath     string             `json:"remote_path"`
	LocalMetadata  FileMetadata       `json:"local_metadata"`
	RemoteMetadata RemoteMetadata     `json:"remote_metadata"`
	ConflictType   ConflictType       `json:"conflict_type"`
	Resolution     ConflictResolution `json:"resolution,omitempty"`
	Timestamp      time.Time          `json:"timestamp"`
}

// ConflictType represents the type of conflict
type ConflictType string

const (
	ConflictTypeBothModified  ConflictType = "both_modified"
	ConflictTypeDeletedLocal  ConflictType = "deleted_local"
	ConflictTypeDeletedRemote ConflictType = "deleted_remote"
	ConflictTypeTypeChanged   ConflictType = "type_changed"
)

// SyncConfig represents configuration for sync operations
type SyncConfig struct {
	IncludePatterns    []string           `json:"include_patterns"`
	ExcludePatterns    []string           `json:"exclude_patterns"`
	ConflictResolution ConflictResolution `json:"conflict_resolution"`
	SyncInterval       time.Duration      `json:"sync_interval"`
	MaxRetries         int                `json:"max_retries"`
	WatchMode          bool               `json:"watch_mode"`
}

// ChangeType represents the type of change detected
type ChangeType string

const (
	ChangeTypeCreate ChangeType = "create"
	ChangeTypeModify ChangeType = "modify"
	ChangeTypeDelete ChangeType = "delete"
	ChangeTypeMove   ChangeType = "move"
	ChangeTypeRename ChangeType = "rename"
)

// DetectedChange represents a change detected during state comparison
type DetectedChange struct {
	Type        ChangeType  `json:"type"`
	Path        string      `json:"path"`
	OldPath     string      `json:"old_path,omitempty"`
	IsLocal     bool        `json:"is_local"`
	Metadata    interface{} `json:"metadata"`
	OldMetadata interface{} `json:"old_metadata,omitempty"`
	Timestamp   time.Time   `json:"timestamp"`
}

// MoveCandidate represents a potential move/rename operation
type MoveCandidate struct {
	OldPath    string      `json:"old_path"`
	NewPath    string      `json:"new_path"`
	Confidence float64     `json:"confidence"`
	Reason     string      `json:"reason"`
	IsLocal    bool        `json:"is_local"`
	Metadata   interface{} `json:"metadata"`
}

// StateSnapshot represents a complete snapshot of file system state
type StateSnapshot struct {
	LocalFiles  map[string]FileMetadata   `json:"local_files"`
	RemoteFiles map[string]RemoteMetadata `json:"remote_files"`
	Timestamp   time.Time                 `json:"timestamp"`
	Version     int64                     `json:"version"`
}
