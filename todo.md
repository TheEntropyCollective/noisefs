# NoiseFS Development Todo

## Completed Phases Summary

### Phase 1: FUSE Persistent Index (2025-07-07)
- ✅ Persistent file index with JSON storage (`pkg/fuse/index.go`)
- ✅ CLI index management in `noisefs-mount` 
- ✅ Thread-safe operations with atomic file writes
- ✅ Complete file reading through FUSE filesystem
- ✅ Automatic index loading/saving on mount/unmount

### Phase 2: Write Operations (High Priority)
- [ ] Implement basic write support in NoiseFile.Write()
- [ ] Add file creation support (Create() method)
- [ ] Implement write buffering and flush operations
- [ ] Auto-upload on file close/flush
- [ ] Update index automatically on new file creation

### Phase 3: Enhanced FUSE Features (Medium Priority)
- [ ] Directory operations (Mkdir, Rmdir, Rename)
- [ ] Extended attributes and metadata
- [ ] File locking support
- [ ] Symbolic links

### Phase 4: Performance & Production (Low Priority)
- [ ] Performance benchmarks for FUSE operations
- [ ] Read-ahead and write-back caching
- [ ] Configuration file support
- [ ] Proper logging system
- [ ] Docker containerization