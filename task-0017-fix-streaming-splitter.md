# task-0017 - Fix failing TestStreamingSplitter test

## Description

The TestStreamingSplitter test in pkg/core/blocks/streaming_test.go is currently failing with the error "Reassembled data doesn't match original". This test failure was discovered during task 0011 validation but is unrelated to the manifest update functionality. The issue appears to be in the streaming splitter logic where data is not being properly reassembled from blocks.

## Acceptance Criteria

- [ ] TestStreamingSplitter test passes when run individually
- [ ] TestStreamingSplitter test passes in CI/build pipeline  
- [ ] Root cause of failure identified and documented in implementation notes
- [ ] Fix does not break other streaming/splitter functionality
- [ ] All related streaming and splitter tests continue to pass

## Implementation Plan

(To be added when work begins)

## Implementation Notes

(To be added when work is completed)