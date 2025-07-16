package util

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses a human-readable size string (e.g., "10MB", "1.5GB") into bytes
func ParseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))
	if sizeStr == "" {
		return 0, fmt.Errorf("empty size string")
	}
	
	// Define size units
	units := map[string]int64{
		"B":   1,
		"KB":  1024,
		"KIB": 1024,
		"MB":  1024 * 1024,
		"MIB": 1024 * 1024,
		"GB":  1024 * 1024 * 1024,
		"GIB": 1024 * 1024 * 1024,
		"TB":  1024 * 1024 * 1024 * 1024,
		"TIB": 1024 * 1024 * 1024 * 1024,
	}
	
	// Try to find a unit suffix
	var numberPart string
	var unitPart string
	
	for unit := range units {
		if strings.HasSuffix(sizeStr, unit) {
			numberPart = strings.TrimSuffix(sizeStr, unit)
			unitPart = unit
			break
		}
	}
	
	// If no unit found, assume bytes
	if unitPart == "" {
		n, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid size format: %s", sizeStr)
		}
		return n, nil
	}
	
	// Parse the number part
	numberPart = strings.TrimSpace(numberPart)
	number, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size number: %s", numberPart)
	}
	
	// Calculate bytes
	multiplier := units[unitPart]
	bytes := int64(number * float64(multiplier))
	
	return bytes, nil
}

// FormatSize formats a size in bytes to a human-readable string
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}