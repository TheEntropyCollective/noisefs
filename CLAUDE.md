# CLAUDE.md

- **Workflow Guideline**: 
- YOU MUST ALWAYS FOLLOW the standard workflow
- YOU MUST ALWAYS keep todo.md clean. Just the current milestone, the sprints and tasks for that milestone and the completed milestone

## Standard Workflow 
1. First think hard through the problem at hand (using Opus), read the codebase for relevant files, and write a plan to the "Current Milestone" section in docs/todo.md.
2. The plan should have a list of sprints and todo items in each sprint that you can check off as you complete them
3. Before you begin working, check in with me and I will verify the plan.
4. Then, begin working on the todos for the next sprint (using Sonnet), marking them as complete as you go.
5. Please every step of the way just give me a high level explanation of what changes you made
6. Make every task and code change you do as simple as possible. We want to avoid making any massive or complex changes. Every change should impact as little code as possible. Everything is about simplicity.
7. Git commit after every task (don't include anything about being created with Claude)
8. When you are done with a sprint, update it as completed in todo.md
9. When you complete a milestone, summarize it under "Completed Milestones"
10. Push the completed milestone to github
11. Ask about completing the next sprint or milestone. Suggest additional ideas for other milestones.


## Project Overview

NoiseFS implements the OFFSystem architecture on IPFS for privacy-preserving P2P distributed storage.

## Core Principles

- **Block Anonymization**: XOR operations make blocks appear as random data
- **Multi-use Blocks**: Each block serves multiple files for plausible deniability
- **Direct Retrieval**: No forwarding through intermediate nodes
- **No Original Content**: Only anonymized blocks are stored

## Architecture

- **Block Layer** (`pkg/core/blocks/`): Splitting and anonymization
- **Storage Layer** (`pkg/ipfs/`, `pkg/storage/`): IPFS integration and backend management
- **Metadata Layer** (`pkg/core/descriptors/`): File reconstruction descriptors
- **Cache Layer** (`pkg/storage/cache/`): Block reuse optimization

## Goals

- <200% storage overhead (vs 900-2900% for traditional anonymous systems)
- Streaming support for large files
- Strong privacy guarantees with plausible deniability
- Efficient block reuse through smart caching

## Package Documentation

Implementation details are in package-specific CLAUDE.md files:
- [Core Blocks](pkg/core/blocks/CLAUDE.md) - Block splitting and XOR operations
- [Storage](pkg/storage/CLAUDE.md) - Storage backends and management
- [Cache](pkg/storage/cache/CLAUDE.md) - Caching strategies and optimization
- [Descriptors](pkg/core/descriptors/CLAUDE.md) - File metadata handling
- [IPFS](pkg/ipfs/CLAUDE.md) - IPFS integration specifics
