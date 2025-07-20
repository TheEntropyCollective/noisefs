---
id: task-0011
title: Implement directory manifest updates after file changes
status: To Do
assignee: []
created_date: '2025-07-20'
labels: []
dependencies: []
---

## Description

Update parent directory manifests when files are added, modified, or deleted to maintain accurate directory structure in NoiseFS. This ensures remote directory browsing reflects current state.

## Acceptance Criteria

- [ ] Directory manifests are updated after file operations
- [ ] Changes propagate up the directory tree
- [ ] New manifest CIDs are tracked after updates
- [ ] Concurrent updates to same directory are handled safely
- [ ] Failed manifest updates trigger appropriate retry logic
