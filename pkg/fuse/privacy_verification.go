// +build fuse

package fuse

import (
	"strings"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// VerifyPrivacyCompliance checks that extended attributes don't expose sensitive metadata
func VerifyPrivacyCompliance(fs *NoiseFS, filename string) []string {
	var violations []string
	context := &fuse.Context{}
	
	// Test ListXAttr for privacy violations
	attrs, status := fs.ListXAttr(filename, context)
	if status == fuse.OK {
		for _, attr := range attrs {
			// Check for sensitive attributes that should be removed
			if strings.Contains(attr, "descriptor_cid") {
				violations = append(violations, "descriptor_cid attribute exposes content identifiers")
			}
			if strings.Contains(attr, "created_at") || strings.Contains(attr, "modified_at") {
				violations = append(violations, "timestamp attributes enable timing correlation attacks")
			}
			if strings.Contains(attr, "directory") {
				violations = append(violations, "directory attribute exposes path information")
			}
			if strings.Contains(attr, "file_size") {
				violations = append(violations, "file_size attribute may aid fingerprinting")
			}
		}
	}
	
	// Test specific sensitive attributes directly
	sensitiveAttrs := []string{
		"user.noisefs.descriptor_cid",
		"user.noisefs.created_at", 
		"user.noisefs.modified_at",
		"user.noisefs.file_size",
		"user.noisefs.directory",
	}
	
	for _, attr := range sensitiveAttrs {
		data, status := fs.GetXAttr(filename, attr, context)
		if status == fuse.OK && len(data) > 0 {
			violations = append(violations, "sensitive attribute "+attr+" is accessible")
		}
	}
	
	return violations
}

// GetSafeAttributes returns the list of privacy-safe attributes that should be available
func GetSafeAttributes() []string {
	return []string{
		"user.noisefs.type",
		"user.noisefs.version",
		"user.noisefs.encrypted",
	}
}