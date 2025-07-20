---
id: task-0012
title: Add bidirectional sync state tracking
status: To Do
assignee: []
created_date: '2025-07-20'
labels: []
dependencies: []
---

## Description

Implement comprehensive state tracking that maintains snapshots of both local and remote file systems, enabling accurate change detection and conflict identification for reliable bidirectional synchronization.

## Acceptance Criteria

- [ ] Local file snapshots include checksums and metadata
- [ ] Remote snapshots track CIDs and modification times
- [ ] State comparison accurately detects all change types
- [ ] Move and rename operations are detected efficiently
- [ ] State updates are atomic to prevent corruption
