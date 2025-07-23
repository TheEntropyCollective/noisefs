---
id: task-0010
title: Add remote path to CID mapping persistence
status: To Do
assignee: []
created_date: '2025-07-20'
updated_date: '2025-07-21'
labels: []
dependencies:
  - task-0012
---

## Description

Implement persistent storage of mappings between remote file paths and their descriptor CIDs to enable efficient lookups during sync operations and change detection.

## Acceptance Criteria

- [ ] Path to CID mappings are stored in sync state
- [ ] Mappings are updated after successful uploads
- [ ] Download operations use stored mappings
- [ ] Mappings persist across sync sessions
- [ ] Old mappings are cleaned up when files are deleted
