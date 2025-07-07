// +build fuse

package fuse

import (
	"context"
	"os"
	"sync"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

// FS implements the FUSE filesystem interface for NoiseFS
type FS struct {
	client      *noisefs.Client
	mountPath   string
	fileManager *FileManager
	
	// Virtual filesystem structure
	mu    sync.RWMutex
	nodes map[uint64]*Node // inode -> node mapping
	
	// Root directories
	filesRoot       *Dir
	cacheRoot       *Dir  
	descriptorsRoot *Dir
	metaRoot        *Dir
	
	nextInode uint64
}

// NewFS creates a new FUSE filesystem for NoiseFS
func NewFS(client *noisefs.Client, mountPath string) *FS {
	fs := &FS{
		client:      client,
		mountPath:   mountPath,
		fileManager: NewFileManager(client),
		nodes:       make(map[uint64]*Node),
		nextInode:   1,
	}
	
	// Create root directory structure
	fs.setupRootStructure()
	
	return fs
}

// setupRootStructure creates the initial virtual directory structure
func (fs *FS) setupRootStructure() {
	// Create root directories
	fs.filesRoot = NewDir(fs, "files", fs.nextInode())
	fs.cacheRoot = NewDir(fs, "cache", fs.nextInode())
	fs.descriptorsRoot = NewDir(fs, "descriptors", fs.nextInode())
	fs.metaRoot = NewDir(fs, ".noisefs", fs.nextInode())
	
	// Register root nodes
	fs.nodes[fs.filesRoot.inode] = &Node{
		Type: NodeTypeDir,
		Dir:  fs.filesRoot,
	}
	fs.nodes[fs.cacheRoot.inode] = &Node{
		Type: NodeTypeDir,
		Dir:  fs.cacheRoot,
	}
	fs.nodes[fs.descriptorsRoot.inode] = &Node{
		Type: NodeTypeDir,
		Dir:  fs.descriptorsRoot,
	}
	fs.nodes[fs.metaRoot.inode] = &Node{
		Type: NodeTypeDir,
		Dir:  fs.metaRoot,
	}
}

// nextInode generates the next available inode number
func (fs *FS) nextInode() uint64 {
	fs.nextInode++
	return fs.nextInode
}

// Root returns the root directory of the filesystem
func (fs *FS) Root() (fs.Node, error) {
	return &RootDir{fs: fs}, nil
}

// Statfs returns filesystem statistics
func (fs *FS) Statfs(ctx context.Context, req *fuse.StatfsRequest, resp *fuse.StatfsResponse) error {
	// Provide basic filesystem statistics
	resp.Blocks = 1000000    // Total blocks
	resp.Bfree = 900000      // Free blocks  
	resp.Bavail = 900000     // Available blocks
	resp.Files = 1000000     // Total inodes
	resp.Ffree = 999000      // Free inodes
	resp.Bsize = 4096        // Block size
	resp.Namelen = 255       // Max filename length
	resp.Frsize = 4096       // Fragment size
	
	return nil
}

// NodeType represents the type of filesystem node
type NodeType int

const (
	NodeTypeDir NodeType = iota
	NodeTypeFile
)

// Node represents a filesystem node (file or directory)
type Node struct {
	Type NodeType
	Dir  *Dir
	File *File
}

// RootDir represents the root directory of the filesystem
type RootDir struct {
	fs *FS
}

// Attr sets the attributes for the root directory
func (d *RootDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = 1
	attr.Mode = os.ModeDir | 0755
	attr.Uid = uint32(os.Getuid())
	attr.Gid = uint32(os.Getgid())
	return nil
}

// Lookup finds a child node in the root directory
func (d *RootDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	d.fs.mu.RLock()
	defer d.fs.mu.RUnlock()
	
	switch name {
	case "files":
		return d.fs.filesRoot, nil
	case "cache":
		return d.fs.cacheRoot, nil
	case "descriptors":
		return d.fs.descriptorsRoot, nil
	case ".noisefs":
		return d.fs.metaRoot, nil
	default:
		return nil, syscall.ENOENT
	}
}

// ReadDirAll returns all entries in the root directory
func (d *RootDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return []fuse.Dirent{
		{Inode: d.fs.filesRoot.inode, Name: "files", Type: fuse.DT_Dir},
		{Inode: d.fs.cacheRoot.inode, Name: "cache", Type: fuse.DT_Dir},
		{Inode: d.fs.descriptorsRoot.inode, Name: "descriptors", Type: fuse.DT_Dir},
		{Inode: d.fs.metaRoot.inode, Name: ".noisefs", Type: fuse.DT_Dir},
	}, nil
}

// Mkdir creates a new directory in the root
func (d *RootDir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	// Only allow creation of subdirectories in specific locations
	return nil, syscall.EPERM
}

// Create creates a new file in the root directory
func (d *RootDir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	// Files should be created in the files/ directory, not root
	return nil, nil, syscall.EPERM
}

// Remove removes a file or directory from the root
func (d *RootDir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	// Don't allow removal of root directories
	return syscall.EPERM
}

// Interface check
var _ fs.FS = (*FS)(nil)
var _ fs.Node = (*RootDir)(nil)
var _ fs.NodeStringLookuper = (*RootDir)(nil)
var _ fs.HandleReadDirAller = (*RootDir)(nil)
var _ fs.NodeMkdirer = (*RootDir)(nil)
var _ fs.NodeCreater = (*RootDir)(nil)
var _ fs.NodeRemover = (*RootDir)(nil)