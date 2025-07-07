// +build fuse

package fuse

import (
	"context"
	"os"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// Dir represents a directory in the NoiseFS FUSE filesystem
type Dir struct {
	fs    *FS
	name  string
	inode uint64
	
	// Directory contents
	mu       sync.RWMutex
	children map[string]*Node
	
	// Metadata
	mode     os.FileMode
	uid, gid uint32
	ctime    time.Time
	mtime    time.Time
}

// NewDir creates a new directory node
func NewDir(fs *FS, name string, inode uint64) *Dir {
	now := time.Now()
	return &Dir{
		fs:       fs,
		name:     name,
		inode:    inode,
		children: make(map[string]*Node),
		mode:     os.ModeDir | 0755,
		uid:      uint32(os.Getuid()),
		gid:      uint32(os.Getgid()),
		ctime:    now,
		mtime:    now,
	}
}

// Attr sets the attributes for the directory
func (d *Dir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = d.mode
	attr.Uid = d.uid
	attr.Gid = d.gid
	attr.Ctime = d.ctime
	attr.Mtime = d.mtime
	attr.Atime = time.Now() // Access time is now
	
	// Directory size is based on number of entries
	d.mu.RLock()
	attr.Size = uint64(len(d.children) * 32) // Rough estimate
	d.mu.RUnlock()
	
	return nil
}

// Lookup finds a child node by name
func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	child, exists := d.children[name]
	if !exists {
		// For the files directory, try to load from NoiseFS
		if d.name == "files" {
			return d.loadFileFromNoiseFS(name)
		}
		return nil, syscall.ENOENT
	}
	
	switch child.Type {
	case NodeTypeDir:
		return child.Dir, nil
	case NodeTypeFile:
		return child.File, nil
	default:
		return nil, syscall.ENOENT
	}
}

// loadFileFromNoiseFS attempts to load a file from NoiseFS descriptors
func (d *Dir) loadFileFromNoiseFS(name string) (fs.Node, error) {
	// Try to load file using FileManager
	file, err := d.fs.fileManager.LoadFile(name)
	if err != nil {
		return nil, syscall.ENOENT
	}
	
	// Set filesystem reference and inode
	file.fs = d.fs
	file.inode = d.fs.nextInode()
	
	// Add to directory children
	d.children[name] = &Node{
		Type: NodeTypeFile,
		File: file,
	}
	
	// Register in filesystem
	d.fs.mu.Lock()
	d.fs.nodes[file.inode] = d.children[name]
	d.fs.mu.Unlock()
	
	// Register file path
	fullPath := d.getFullPath() + "/" + name
	d.fs.fileManager.RegisterFilePath(fullPath, name)
	
	return file, nil
}

// ReadDirAll returns all directory entries
func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	entries := make([]fuse.Dirent, 0, len(d.children))
	
	for name, child := range d.children {
		entry := fuse.Dirent{
			Name: name,
		}
		
		switch child.Type {
		case NodeTypeDir:
			entry.Inode = child.Dir.inode
			entry.Type = fuse.DT_Dir
		case NodeTypeFile:
			entry.Inode = child.File.inode
			entry.Type = fuse.DT_File
		}
		
		entries = append(entries, entry)
	}
	
	return entries, nil
}

// Mkdir creates a new subdirectory
func (d *Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Check if name already exists
	if _, exists := d.children[req.Name]; exists {
		return nil, syscall.EEXIST
	}
	
	// Only allow subdirectories in the files directory
	if d.name != "files" {
		return nil, syscall.EPERM
	}
	
	// Create new directory
	newDir := NewDir(d.fs, req.Name, d.fs.nextInode())
	newDir.mode = req.Mode | os.ModeDir
	newDir.uid = req.Uid
	newDir.gid = req.Gid
	
	// Add to children
	d.children[req.Name] = &Node{
		Type: NodeTypeDir,
		Dir:  newDir,
	}
	
	// Register in filesystem
	d.fs.mu.Lock()
	d.fs.nodes[newDir.inode] = d.children[req.Name]
	d.fs.mu.Unlock()
	
	// Update mtime
	d.mtime = time.Now()
	
	return newDir, nil
}

// Create creates a new file in the directory
func (d *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	// Check if name already exists
	if _, exists := d.children[req.Name]; exists {
		return nil, nil, syscall.EEXIST
	}
	
	// Only allow file creation in files directory and subdirectories
	if d.name != "files" && d.fs.filesRoot != d && !d.isUnderFiles() {
		return nil, nil, syscall.EPERM
	}
	
	// Create new file
	newFile := NewFile(d.fs, req.Name, d.fs.nextInode())
	newFile.mode = req.Mode
	newFile.uid = req.Uid
	newFile.gid = req.Gid
	
	// Add to children
	d.children[req.Name] = &Node{
		Type: NodeTypeFile,
		File: newFile,
	}
	
	// Register in filesystem
	d.fs.mu.Lock()
	d.fs.nodes[newFile.inode] = d.children[req.Name]
	d.fs.mu.Unlock()
	
	// Update mtime
	d.mtime = time.Now()
	
	// Return file and handle for writing
	handle := newFile.newHandle()
	resp.Handle = handle.id
	resp.Flags = fuse.OpenNonSeekable // Files are written sequentially
	
	return newFile, handle, nil
}

// Remove removes a file or directory
func (d *Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	child, exists := d.children[req.Name]
	if !exists {
		return syscall.ENOENT
	}
	
	// Only allow removal in files directory and subdirectories
	if d.name != "files" && d.fs.filesRoot != d && !d.isUnderFiles() {
		return syscall.EPERM
	}
	
	// For directories, check if empty
	if child.Type == NodeTypeDir && req.Dir {
		child.Dir.mu.RLock()
		empty := len(child.Dir.children) == 0
		child.Dir.mu.RUnlock()
		
		if !empty {
			return syscall.ENOTEMPTY
		}
	}
	
	// Remove from children
	delete(d.children, req.Name)
	
	// Remove from filesystem nodes
	var inode uint64
	switch child.Type {
	case NodeTypeDir:
		inode = child.Dir.inode
	case NodeTypeFile:
		inode = child.File.inode
		// TODO: Mark file for deletion from NoiseFS
	}
	
	d.fs.mu.Lock()
	delete(d.fs.nodes, inode)
	d.fs.mu.Unlock()
	
	// Update mtime
	d.mtime = time.Now()
	
	return nil
}

// isUnderFiles checks if this directory is a subdirectory of the files root
func (d *Dir) isUnderFiles() bool {
	// This is a simplified check - in a full implementation we'd walk up the tree
	// For now, assume any directory that's not a root directory is under files
	return d != d.fs.filesRoot && d != d.fs.cacheRoot && 
		   d != d.fs.descriptorsRoot && d != d.fs.metaRoot
}

// getFullPath returns the full path of this directory
func (d *Dir) getFullPath() string {
	if d == d.fs.filesRoot {
		return "/files"
	} else if d == d.fs.cacheRoot {
		return "/cache"
	} else if d == d.fs.descriptorsRoot {
		return "/descriptors"
	} else if d == d.fs.metaRoot {
		return "/.noisefs"
	}
	
	// For subdirectories, this is simplified - in a full implementation
	// we'd walk up the tree to build the full path
	return "/files/" + d.name
}

// Interface check
var _ fs.Node = (*Dir)(nil)
var _ fs.NodeStringLookuper = (*Dir)(nil)
var _ fs.HandleReadDirAller = (*Dir)(nil)
var _ fs.NodeMkdirer = (*Dir)(nil)
var _ fs.NodeCreater = (*Dir)(nil)
var _ fs.NodeRemover = (*Dir)(nil)