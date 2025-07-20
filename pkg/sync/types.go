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
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
	IsDir        bool      `json:"is_dir"`
	Checksum     string    `json:"checksum,omitempty"`
	Permissions  uint32    `json:"permissions"`
}

// RemoteMetadata represents metadata for a remote file in NoiseFS
type RemoteMetadata struct {
	Path           string    `json:"path"`
	DescriptorCID  string    `json:"descriptor_cid"`
	Size           int64     `json:"size"`
	ModTime        time.Time `json:"mod_time"`
	IsDir          bool      `json:"is_dir"`
	EncryptionKey  string    `json:"encryption_key,omitempty"`
}

// SyncOperation represents a sync operation to be performed
type SyncOperation struct {
	ID        string        `json:"id"`
	Type      OperationType `json:"type"`
	LocalPath string        `json:"local_path"`
	RemotePath string       `json:"remote_path"`
	Timestamp time.Time     `json:"timestamp"`
	Status    OpStatus      `json:"status"`
	Retries   int           `json:"retries"`
	Error     string        `json:"error,omitempty"`
}

// OperationType represents the type of sync operation
type OperationType string

const (
	OpTypeUpload     OperationType = "upload"
	OpTypeDownload   OperationType = "download"
	OpTypeDelete     OperationType = "delete"
	OpTypeCreateDir  OperationType = "create_dir"
	OpTypeDeleteDir  OperationType = "delete_dir"
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
	ManifestCID    string                    `json:"manifest_cid,omitempty"`    // NoiseFS directory manifest CID
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
	ConflictTypeBothModified ConflictType = "both_modified"
	ConflictTypeDeletedLocal ConflictType = "deleted_local"
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