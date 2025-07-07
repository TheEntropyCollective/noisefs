// +build fuse

package fuse

import (
	"context"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// File represents a file in the NoiseFS FUSE filesystem
type File struct {
	fs    *FS
	name  string
	inode uint64
	
	// File content and state
	mu       sync.RWMutex
	data     []byte
	size     uint64
	uploaded bool
	
	// NoiseFS metadata
	descriptorCID string
	
	// File metadata
	mode     os.FileMode
	uid, gid uint32
	ctime    time.Time
	mtime    time.Time
	atime    time.Time
	
	// Handle management
	handleMu    sync.Mutex
	nextHandle  uint64
	openHandles map[uint64]*FileHandle
}

// NewFile creates a new file node
func NewFile(fs *FS, name string, inode uint64) *File {
	now := time.Now()
	return &File{
		fs:          fs,
		name:        name,
		inode:       inode,
		data:        make([]byte, 0),
		mode:        0644,
		uid:         uint32(os.Getuid()),
		gid:         uint32(os.Getgid()),
		ctime:       now,
		mtime:       now,
		atime:       now,
		openHandles: make(map[uint64]*FileHandle),
	}
}

// Attr sets the attributes for the file
func (f *File) Attr(ctx context.Context, attr *fuse.Attr) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	attr.Inode = f.inode
	attr.Mode = f.mode
	attr.Uid = f.uid
	attr.Gid = f.gid
	attr.Size = f.size
	attr.Ctime = f.ctime
	attr.Mtime = f.mtime
	attr.Atime = f.atime
	attr.Blocks = (f.size + 511) / 512 // 512-byte blocks
	
	return nil
}

// Open opens the file for reading or writing
func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	// Update access time
	f.mu.Lock()
	f.atime = time.Now()
	f.mu.Unlock()
	
	// Create new handle
	handle := f.newHandle()
	
	// If file hasn't been loaded from NoiseFS yet, load it
	if !f.uploaded && f.descriptorCID != "" {
		err := f.loadFromNoiseFS()
		if err != nil {
			return nil, err
		}
	}
	
	resp.Handle = handle.id
	resp.Flags = fuse.OpenKeepCache // Allow kernel to cache
	
	return handle, nil
}

// loadFromNoiseFS loads file content from NoiseFS using the descriptor CID
func (f *File) loadFromNoiseFS() error {
	// This will be implemented when we integrate with descriptors
	// For now, return success for empty files
	return nil
}

// newHandle creates a new file handle
func (f *File) newHandle() *FileHandle {
	f.handleMu.Lock()
	defer f.handleMu.Unlock()
	
	handleID := atomic.AddUint64(&f.nextHandle, 1)
	handle := &FileHandle{
		id:   handleID,
		file: f,
	}
	
	f.openHandles[handleID] = handle
	return handle
}

// removeHandle removes a file handle
func (f *File) removeHandle(handleID uint64) {
	f.handleMu.Lock()
	defer f.handleMu.Unlock()
	
	delete(f.openHandles, handleID)
}

// FileHandle represents an open file handle
type FileHandle struct {
	id   uint64
	file *File
	
	// Handle state
	mu     sync.Mutex
	offset int64
	dirty  bool
}

// Read reads data from the file
func (fh *FileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	
	fh.file.mu.RLock()
	defer fh.file.mu.RUnlock()
	
	// Update access time
	fh.file.atime = time.Now()
	
	// Calculate read parameters
	offset := req.Offset
	size := int64(len(fh.file.data))
	
	if offset >= size {
		// EOF
		resp.Data = resp.Data[:0]
		return nil
	}
	
	// Calculate how much to read
	remaining := size - offset
	toRead := int64(req.Size)
	if toRead > remaining {
		toRead = remaining
	}
	
	// Read data
	data := make([]byte, toRead)
	copy(data, fh.file.data[offset:offset+toRead])
	resp.Data = data
	
	return nil
}

// Write writes data to the file
func (fh *FileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	
	fh.file.mu.Lock()
	defer fh.file.mu.Unlock()
	
	// Calculate write parameters
	offset := req.Offset
	data := req.Data
	dataLen := int64(len(data))
	
	// Expand file if necessary
	requiredSize := offset + dataLen
	if requiredSize > int64(len(fh.file.data)) {
		newData := make([]byte, requiredSize)
		copy(newData, fh.file.data)
		fh.file.data = newData
	}
	
	// Write data
	copy(fh.file.data[offset:offset+dataLen], data)
	
	// Update file size and metadata
	if requiredSize > int64(fh.file.size) {
		fh.file.size = uint64(requiredSize)
	}
	fh.file.mtime = time.Now()
	fh.file.uploaded = false // Mark as needing upload
	fh.dirty = true
	
	resp.Size = len(data)
	return nil
}

// Flush flushes any pending writes to storage
func (fh *FileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	
	if !fh.dirty {
		return nil
	}
	
	// Upload file to NoiseFS
	err := fh.uploadToNoiseFS()
	if err != nil {
		return err
	}
	
	fh.dirty = false
	return nil
}

// uploadToNoiseFS uploads the file content to NoiseFS
func (fh *FileHandle) uploadToNoiseFS() error {
	fh.file.mu.RLock()
	data := make([]byte, len(fh.file.data))
	copy(data, fh.file.data)
	fh.file.mu.RUnlock()
	
	// This will be implemented when we integrate with NoiseFS client
	// For now, just mark as uploaded
	fh.file.mu.Lock()
	fh.file.uploaded = true
	fh.file.mu.Unlock()
	
	return nil
}

// Release closes the file handle
func (fh *FileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	// Flush any pending writes
	if fh.dirty {
		err := fh.uploadToNoiseFS()
		if err != nil {
			// Log error but don't fail release
			// TODO: Add proper logging
		}
	}
	
	// Remove handle from file
	fh.file.removeHandle(fh.id)
	
	return nil
}

// Fsync synchronizes file content to storage
func (fh *FileHandle) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	return fh.Flush(ctx, &fuse.FlushRequest{})
}

// Setattr sets file attributes
func (f *File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Handle size changes (truncation)
	if req.Valid.Size() {
		newSize := req.Size
		if newSize != f.size {
			if newSize > f.size {
				// Extend file with zeros
				extended := make([]byte, newSize)
				copy(extended, f.data)
				f.data = extended
			} else {
				// Truncate file
				f.data = f.data[:newSize]
			}
			f.size = newSize
			f.mtime = time.Now()
			f.uploaded = false
		}
	}
	
	// Handle mode changes
	if req.Valid.Mode() {
		f.mode = req.Mode
	}
	
	// Handle time changes
	if req.Valid.Mtime() {
		f.mtime = req.Mtime
	}
	if req.Valid.Atime() {
		f.atime = req.Atime
	}
	
	// Return current attributes
	return f.Attr(ctx, &resp.Attr)
}

// Interface check
var _ fs.Node = (*File)(nil)
var _ fs.NodeOpener = (*File)(nil)
var _ fs.NodeSetattrer = (*File)(nil)
var _ fs.Handle = (*FileHandle)(nil)
var _ fs.HandleReader = (*FileHandle)(nil)
var _ fs.HandleWriter = (*FileHandle)(nil)
var _ fs.HandleFlusher = (*FileHandle)(nil)
var _ fs.HandleReleaser = (*FileHandle)(nil)
var _ fs.HandleFsyncer = (*FileHandle)(nil)