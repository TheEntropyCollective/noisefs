package tags

import (
	"fmt"
	"strconv"
	"strings"
)

// Tag represents a parsed tag with namespace and value
type Tag struct {
	Namespace string // e.g., "res", "genre", "year"
	Value     string // e.g., "4k", "scifi", "2023"
	Raw       string // Original tag string
}

// TagParser handles parsing and validation of tags
type TagParser struct {
	validators map[string]TagValidator
}

// TagValidator validates tag values for a specific namespace
type TagValidator func(value string) error

// NewTagParser creates a new tag parser with default validators
func NewTagParser() *TagParser {
	return &TagParser{
		validators: getDefaultValidators(),
	}
}

// Parse parses a tag string into a Tag struct
func (p *TagParser) Parse(tagStr string) (*Tag, error) {
	tagStr = strings.TrimSpace(tagStr)
	if tagStr == "" {
		return nil, fmt.Errorf("empty tag")
	}
	
	// Check for namespace:value format
	parts := strings.SplitN(tagStr, ":", 2)
	
	if len(parts) == 2 {
		namespace := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		
		// Validate if validator exists
		if validator, exists := p.validators[namespace]; exists {
			if err := validator(value); err != nil {
				return nil, fmt.Errorf("invalid %s tag: %w", namespace, err)
			}
		}
		
		return &Tag{
			Namespace: namespace,
			Value:     value,
			Raw:       tagStr,
		}, nil
	}
	
	// Plain tag without namespace
	return &Tag{
		Namespace: "",
		Value:     tagStr,
		Raw:       tagStr,
	}, nil
}

// ParseMultiple parses multiple tag strings
func (p *TagParser) ParseMultiple(tagStrs []string) ([]*Tag, error) {
	tags := make([]*Tag, 0, len(tagStrs))
	
	for _, tagStr := range tagStrs {
		tag, err := p.Parse(tagStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tag '%s': %w", tagStr, err)
		}
		tags = append(tags, tag)
	}
	
	return tags, nil
}

// AddValidator adds a custom validator for a namespace
func (p *TagParser) AddValidator(namespace string, validator TagValidator) {
	p.validators[strings.ToLower(namespace)] = validator
}

// Format formats a tag back to string
func (t *Tag) Format() string {
	if t.Namespace != "" {
		return fmt.Sprintf("%s:%s", t.Namespace, t.Value)
	}
	return t.Value
}

// IsNamespaced returns true if the tag has a namespace
func (t *Tag) IsNamespaced() bool {
	return t.Namespace != ""
}

// Default validators

func getDefaultValidators() map[string]TagValidator {
	return map[string]TagValidator{
		"res":      validateResolution,
		"fps":      validateFPS,
		"year":     validateYear,
		"size":     validateSize,
		"bitrate":  validateBitrate,
		"pages":    validatePages,
		"duration": validateDuration,
		"quality":  validateQuality,
	}
}

func validateResolution(value string) error {
	// Allow wildcard for matching
	if value == "*" {
		return nil
	}
	
	validResolutions := map[string]bool{
		"480p": true, "720p": true, "1080p": true,
		"1440p": true, "4k": true, "8k": true,
		"sd": true, "hd": true, "fhd": true, "uhd": true,
	}
	
	lower := strings.ToLower(value)
	if validResolutions[lower] {
		return nil
	}
	
	// Check for WxH format
	if strings.Contains(value, "x") {
		parts := strings.Split(value, "x")
		if len(parts) == 2 {
			if _, err := strconv.Atoi(parts[0]); err != nil {
				return fmt.Errorf("invalid width")
			}
			if _, err := strconv.Atoi(parts[1]); err != nil {
				return fmt.Errorf("invalid height")
			}
			return nil
		}
	}
	
	return fmt.Errorf("invalid resolution format")
}

func validateFPS(value string) error {
	fps, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	
	if fps < 1 || fps > 240 {
		return fmt.Errorf("must be between 1 and 240")
	}
	
	return nil
}

func validateYear(value string) error {
	// Allow single year
	year, err := strconv.Atoi(value)
	if err == nil {
		if year < 1800 || year > 2100 {
			return fmt.Errorf("year out of range")
		}
		return nil
	}
	
	// Allow decade format (1980s)
	if strings.HasSuffix(value, "s") {
		decade := strings.TrimSuffix(value, "s")
		year, err := strconv.Atoi(decade)
		if err != nil {
			return fmt.Errorf("invalid decade format")
		}
		if year < 1800 || year > 2090 {
			return fmt.Errorf("decade out of range")
		}
		return nil
	}
	
	// Allow year range (2000-2010)
	if strings.Contains(value, "-") {
		parts := strings.Split(value, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid year range")
		}
		
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid year range")
		}
		
		if start > end || start < 1800 || end > 2100 {
			return fmt.Errorf("invalid year range")
		}
		
		return nil
	}
	
	return fmt.Errorf("invalid year format")
}

func validateSize(value string) error {
	validSizes := map[string]bool{
		"tiny": true, "small": true, "medium": true,
		"large": true, "huge": true,
	}
	
	lower := strings.ToLower(value)
	if validSizes[lower] {
		return nil
	}
	
	// Allow size with unit (10mb, 1gb)
	units := []string{"b", "kb", "mb", "gb", "tb"}
	for _, unit := range units {
		if strings.HasSuffix(lower, unit) {
			numStr := strings.TrimSuffix(lower, unit)
			if _, err := strconv.ParseFloat(numStr, 64); err != nil {
				return fmt.Errorf("invalid size number")
			}
			return nil
		}
	}
	
	return fmt.Errorf("invalid size format")
}

func validateBitrate(value string) error {
	// Allow "lossless"
	if strings.ToLower(value) == "lossless" {
		return nil
	}
	
	// Check for number with optional 'k' suffix
	numStr := value
	if strings.HasSuffix(strings.ToLower(value), "k") {
		numStr = value[:len(value)-1]
	}
	
	bitrate, err := strconv.Atoi(numStr)
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	
	if bitrate < 1 || bitrate > 50000 {
		return fmt.Errorf("bitrate out of reasonable range")
	}
	
	return nil
}

func validatePages(value string) error {
	pages, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	
	if pages < 1 {
		return fmt.Errorf("must be positive")
	}
	
	return nil
}

func validateDuration(value string) error {
	// Allow HH:MM:SS format
	if strings.Count(value, ":") == 2 {
		parts := strings.Split(value, ":")
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				return fmt.Errorf("invalid time format")
			}
		}
		return nil
	}
	
	// Allow MM:SS format
	if strings.Count(value, ":") == 1 {
		parts := strings.Split(value, ":")
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				return fmt.Errorf("invalid time format")
			}
		}
		return nil
	}
	
	// Allow seconds only
	if _, err := strconv.Atoi(value); err != nil {
		return fmt.Errorf("invalid duration format")
	}
	
	return nil
}

func validateQuality(value string) error {
	validQualities := map[string]bool{
		"cam": true, "ts": true, "scr": true,
		"dvd": true, "dvdrip": true, "hdtv": true,
		"web": true, "webrip": true, "webdl": true,
		"bluray": true, "bdrip": true, "remux": true,
	}
	
	if validQualities[strings.ToLower(value)] {
		return nil
	}
	
	return fmt.Errorf("unknown quality format")
}

// ExtractNamespace extracts all tags with a specific namespace
func ExtractNamespace(tags []*Tag, namespace string) []string {
	namespace = strings.ToLower(namespace)
	values := []string{}
	
	for _, tag := range tags {
		if tag.Namespace == namespace {
			values = append(values, tag.Value)
		}
	}
	
	return values
}

// GroupByNamespace groups tags by their namespace
func GroupByNamespace(tags []*Tag) map[string][]string {
	groups := make(map[string][]string)
	
	for _, tag := range tags {
		ns := tag.Namespace
		if ns == "" {
			ns = "untagged"
		}
		groups[ns] = append(groups[ns], tag.Value)
	}
	
	return groups
}