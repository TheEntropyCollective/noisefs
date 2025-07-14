// +build !fuse

package fuse

import (
	"errors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

// Stub implementations for when FUSE is not available

// MountOptions contains options for mounting the filesystem
type MountOptions struct {
	MountPath   string
	VolumeName  string
	ReadOnly    bool
	AllowOther  bool
	Debug       bool
}

// MountInfo contains information about mounted filesystems
type MountInfo struct {
	MountPath  string
	VolumeName string
	ReadOnly   bool
	PID        int
}

// Mount is a stub implementation when FUSE is not available
func Mount(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// Unmount is a stub implementation when FUSE is not available
func Unmount(mountPath string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// ListMounts is a stub implementation when FUSE is not available
func ListMounts() ([]MountInfo, error) {
	return nil, errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// Daemon is a stub implementation when FUSE is not available
func Daemon(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions, pidFile string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// MountWithIndex is a stub implementation when FUSE is not available
func MountWithIndex(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions, indexPath string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// DaemonWithIndex is a stub implementation when FUSE is not available
func DaemonWithIndex(client *noisefs.Client, storageManager *storage.Manager, opts MountOptions, pidFile, indexPath string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// StopDaemon is a stub implementation when FUSE is not available
func StopDaemon(pidFile string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}