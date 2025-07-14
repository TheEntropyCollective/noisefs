# CLAUDE.md Documentation Validation Checklist

## Content Preservation
- [ ] All core principles from original are preserved
- [ ] No critical information was lost in migration
- [ ] Implementation details moved to appropriate packages
- [ ] Standard workflow remains intact

## Structure Validation
- [ ] Global CLAUDE.md is concise (<50 lines)
- [ ] Each major package has its own CLAUDE.md
- [ ] No duplication between global and package docs
- [ ] Clear separation of concerns

## Cross-References
- [ ] Global doc links to all package docs
- [ ] Package docs link back to global
- [ ] All links use correct relative paths
- [ ] No broken references

## Consistency
- [ ] Consistent formatting across all docs
- [ ] Terminology used consistently
- [ ] Package names match directory structure
- [ ] All docs follow the template structure

## Coverage
- [ ] pkg/core/blocks/CLAUDE.md exists
- [ ] pkg/storage/cache/CLAUDE.md exists
- [ ] pkg/core/descriptors/CLAUDE.md exists
- [ ] pkg/storage/CLAUDE.md exists
- [ ] pkg/ipfs/CLAUDE.md exists

## Usability
- [ ] New developer can understand system from global doc
- [ ] Package maintainers have clear local guidance
- [ ] Implementation details are easy to find
- [ ] Navigation between docs is intuitive