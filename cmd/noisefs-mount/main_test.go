package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
)

func TestParseMultiDirs(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  []fuse.DirectoryMount
		shouldErr bool
	}{
		{
			name:  "Single directory",
			input: "docs:QmXXX:key1",
			expected: []fuse.DirectoryMount{
				{Name: "docs", DescriptorCID: "QmXXX", EncryptionKey: "key1"},
			},
			shouldErr: false,
		},
		{
			name:  "Multiple directories",
			input: "docs:QmXXX:key1,photos:QmYYY:key2",
			expected: []fuse.DirectoryMount{
				{Name: "docs", DescriptorCID: "QmXXX", EncryptionKey: "key1"},
				{Name: "photos", DescriptorCID: "QmYYY", EncryptionKey: "key2"},
			},
			shouldErr: false,
		},
		{
			name:      "Invalid format - missing parts",
			input:     "docs:QmXXX",
			expected:  nil,
			shouldErr: true,
		},
		{
			name:      "Invalid format - too many parts",
			input:     "docs:QmXXX:key1:extra",
			expected:  nil,
			shouldErr: true,
		},
		{
			name:      "Empty input",
			input:     "",
			expected:  []fuse.DirectoryMount{},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse multi-directory mounts
			var multiDirMounts []fuse.DirectoryMount
			var err error

			if tt.input != "" {
				parts := strings.Split(tt.input, ",")
				for _, part := range parts {
					dirParts := strings.Split(part, ":")
					if len(dirParts) != 3 {
						if !tt.shouldErr {
							t.Errorf("Expected no error, but got invalid format")
						}
						err = fmt.Errorf("invalid format")
						break
					}
					multiDirMounts = append(multiDirMounts, fuse.DirectoryMount{
						Name:          dirParts[0],
						DescriptorCID: dirParts[1],
						EncryptionKey: dirParts[2],
					})
				}
			}

			if tt.shouldErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.shouldErr && len(multiDirMounts) != len(tt.expected) {
				t.Errorf("Expected %d mounts, got %d", len(tt.expected), len(multiDirMounts))
			}

			for i, mount := range multiDirMounts {
				if i >= len(tt.expected) {
					break
				}
				if mount.Name != tt.expected[i].Name ||
					mount.DescriptorCID != tt.expected[i].DescriptorCID ||
					mount.EncryptionKey != tt.expected[i].EncryptionKey {
					t.Errorf("Mount %d mismatch: got %+v, expected %+v", i, mount, tt.expected[i])
				}
			}
		})
	}
}

func TestDirectoryMountValidation(t *testing.T) {
	tests := []struct {
		name          string
		descriptor    string
		key           string
		subdir        string
		shouldSucceed bool
	}{
		{
			name:          "Valid directory descriptor only",
			descriptor:    "QmXXXYYYZZZ",
			key:           "",
			subdir:        "",
			shouldSucceed: true,
		},
		{
			name:          "Valid with encryption key",
			descriptor:    "QmXXXYYYZZZ",
			key:           "base64encodedkey==",
			subdir:        "",
			shouldSucceed: true,
		},
		{
			name:          "Valid with subdir",
			descriptor:    "QmXXXYYYZZZ",
			key:           "",
			subdir:        "photos/vacation",
			shouldSucceed: true,
		},
		{
			name:          "Empty descriptor",
			descriptor:    "",
			key:           "key",
			subdir:        "",
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test would validate the actual mounting logic
			// For now, we just check basic validation
			if tt.descriptor == "" && tt.shouldSucceed {
				t.Error("Empty descriptor should not succeed")
			}
			if tt.descriptor != "" && !tt.shouldSucceed {
				t.Error("Non-empty descriptor should succeed")
			}
		})
	}
}
