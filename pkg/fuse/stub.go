// +build !fuse

package fuse

import (
	"errors"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
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
func Mount(client *noisefs.Client, opts MountOptions) error {
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
func Daemon(client *noisefs.Client, opts MountOptions, pidFile string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}

// StopDaemon is a stub implementation when FUSE is not available
func StopDaemon(pidFile string) error {
	return errors.New("FUSE support not available - build with 'go build -tags fuse' and ensure macFUSE/FUSE is installed")
}