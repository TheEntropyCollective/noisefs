package tags

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AutoTagger extracts tags from file metadata
type AutoTagger struct {
	ffprobeAvailable bool
	fileCmd          string
}

// NewAutoTagger creates a new auto tagger
func NewAutoTagger() *AutoTagger {
	at := &AutoTagger{}
	
	// Check for ffprobe
	if _, err := exec.LookPath("ffprobe"); err == nil {
		at.ffprobeAvailable = true
	}
	
	// Check for file command
	if _, err := exec.LookPath("file"); err == nil {
		at.fileCmd = "file"
	}
	
	return at
}

// ExtractTags extracts tags from a file
func (at *AutoTagger) ExtractTags(filePath string) ([]string, error) {
	tags := []string{}
	
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	
	// Basic tags from file info
	tags = append(tags, at.extractBasicTags(filePath, fileInfo)...)
	
	// Extension-based tags
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), "."))
	if ext != "" {
		tags = append(tags, "ext:"+ext)
		tags = append(tags, at.extractExtensionTags(ext)...)
	}
	
	// Try to extract media metadata if available
	if at.isMediaFile(ext) && at.ffprobeAvailable {
		mediaTags, err := at.extractMediaTags(filePath)
		if err == nil {
			tags = append(tags, mediaTags...)
		}
	}
	
	// Try to extract from filename patterns
	filenameTags := at.extractFilenamePatterns(filepath.Base(filePath))
	tags = append(tags, filenameTags...)
	
	return deduplicateTags(tags), nil
}

// extractBasicTags extracts tags from file info
func (at *AutoTagger) extractBasicTags(filePath string, info os.FileInfo) []string {
	tags := []string{}
	
	// Size tags
	size := info.Size()
	if size < 1024*1024 {
		tags = append(tags, "size:tiny")
	} else if size < 10*1024*1024 {
		tags = append(tags, "size:small")
	} else if size < 100*1024*1024 {
		tags = append(tags, "size:medium")
	} else if size < 1024*1024*1024 {
		tags = append(tags, "size:large")
	} else {
		tags = append(tags, "size:huge")
	}
	
	// Modification time tags
	modTime := info.ModTime()
	tags = append(tags, fmt.Sprintf("year:%d", modTime.Year()))
	
	// Add decade tag
	decade := (modTime.Year() / 10) * 10
	tags = append(tags, fmt.Sprintf("decade:%ds", decade))
	
	return tags
}

// extractExtensionTags returns tags based on file extension
func (at *AutoTagger) extractExtensionTags(ext string) []string {
	tags := []string{}
	
	// Video extensions
	videoExts := map[string][]string{
		"mp4":  {"container:mp4", "type:video"},
		"mkv":  {"container:matroska", "type:video"},
		"avi":  {"container:avi", "type:video"},
		"mov":  {"container:quicktime", "type:video"},
		"webm": {"container:webm", "type:video"},
		"flv":  {"container:flv", "type:video"},
		"wmv":  {"container:wmv", "type:video"},
		"m4v":  {"container:mp4", "type:video"},
	}
	
	// Audio extensions
	audioExts := map[string][]string{
		"mp3":  {"codec:mp3", "type:audio"},
		"flac": {"codec:flac", "type:audio", "lossless"},
		"opus": {"codec:opus", "type:audio"},
		"ogg":  {"codec:vorbis", "type:audio"},
		"m4a":  {"codec:aac", "type:audio"},
		"wav":  {"codec:pcm", "type:audio", "lossless"},
		"ape":  {"codec:ape", "type:audio", "lossless"},
		"wma":  {"codec:wma", "type:audio"},
	}
	
	// Document extensions
	docExts := map[string][]string{
		"pdf":  {"format:pdf", "type:document"},
		"epub": {"format:epub", "type:document", "ebook"},
		"mobi": {"format:mobi", "type:document", "ebook"},
		"azw":  {"format:azw", "type:document", "ebook"},
		"txt":  {"format:text", "type:document", "plaintext"},
		"doc":  {"format:doc", "type:document"},
		"docx": {"format:docx", "type:document"},
		"odt":  {"format:odt", "type:document"},
	}
	
	// Image extensions
	imageExts := map[string][]string{
		"jpg":  {"format:jpeg", "type:image"},
		"jpeg": {"format:jpeg", "type:image"},
		"png":  {"format:png", "type:image"},
		"gif":  {"format:gif", "type:image"},
		"webp": {"format:webp", "type:image"},
		"bmp":  {"format:bmp", "type:image"},
		"svg":  {"format:svg", "type:image", "vector"},
		"tiff": {"format:tiff", "type:image"},
	}
	
	if extTags, ok := videoExts[ext]; ok {
		tags = append(tags, extTags...)
	} else if extTags, ok := audioExts[ext]; ok {
		tags = append(tags, extTags...)
	} else if extTags, ok := docExts[ext]; ok {
		tags = append(tags, extTags...)
	} else if extTags, ok := imageExts[ext]; ok {
		tags = append(tags, extTags...)
	}
	
	return tags
}

// extractMediaTags uses ffprobe to extract media metadata
func (at *AutoTagger) extractMediaTags(filePath string) ([]string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	var result struct {
		Streams []struct {
			CodecType   string `json:"codec_type"`
			CodecName   string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			FrameRate   string `json:"r_frame_rate"`
			BitRate     string `json:"bit_rate"`
			Channels    int    `json:"channels"`
			SampleRate  string `json:"sample_rate"`
		} `json:"streams"`
		Format struct {
			Duration string            `json:"duration"`
			BitRate  string            `json:"bit_rate"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}
	
	tags := []string{}
	
	// Process streams
	for _, stream := range result.Streams {
		if stream.CodecType == "video" {
			// Resolution
			if stream.Width > 0 && stream.Height > 0 {
				res := at.classifyResolution(stream.Width, stream.Height)
				if res != "" {
					tags = append(tags, "res:"+res)
				}
				
				// Aspect ratio
				ar := at.calculateAspectRatio(stream.Width, stream.Height)
				if ar != "" {
					tags = append(tags, "aspect:"+ar)
				}
			}
			
			// Frame rate
			if stream.FrameRate != "" {
				if fps := at.parseFrameRate(stream.FrameRate); fps > 0 {
					tags = append(tags, fmt.Sprintf("fps:%d", fps))
				}
			}
			
			// Video codec
			if stream.CodecName != "" {
				tags = append(tags, "vcodec:"+stream.CodecName)
			}
		} else if stream.CodecType == "audio" {
			// Audio codec
			if stream.CodecName != "" {
				tags = append(tags, "acodec:"+stream.CodecName)
			}
			
			// Channels
			if stream.Channels > 0 {
				switch stream.Channels {
				case 1:
					tags = append(tags, "audio:mono")
				case 2:
					tags = append(tags, "audio:stereo")
				case 6:
					tags = append(tags, "audio:5.1")
				case 8:
					tags = append(tags, "audio:7.1")
				}
			}
		}
	}
	
	// Duration
	if result.Format.Duration != "" {
		if duration, err := strconv.ParseFloat(result.Format.Duration, 64); err == nil {
			minutes := int(duration / 60)
			if minutes < 10 {
				tags = append(tags, "duration:short")
			} else if minutes < 60 {
				tags = append(tags, "duration:medium")
			} else {
				tags = append(tags, "duration:long")
			}
		}
	}
	
	// Format tags
	for key, value := range result.Format.Tags {
		switch strings.ToLower(key) {
		case "title":
			// Don't expose title for privacy
		case "year", "date":
			if year := at.extractYear(value); year > 0 {
				tags = append(tags, fmt.Sprintf("year:%d", year))
			}
		case "genre":
			tags = append(tags, "genre:"+strings.ToLower(value))
		}
	}
	
	return tags, nil
}

// extractFilenamePatterns extracts tags from filename patterns
func (at *AutoTagger) extractFilenamePatterns(filename string) []string {
	tags := []string{}
	
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	
	// Common quality patterns
	qualityPatterns := map[string]string{
		`(?i)\b(720p|1080p|1440p|2160p|4k|8k)\b`: "res:$1",
		`(?i)\b(dvdrip|bdrip|webrip|hdtv|webdl|bluray)\b`: "quality:$1",
		`(?i)\b(x264|x265|h264|h265|hevc)\b`: "vcodec:$1",
		`(?i)\b(aac|ac3|dts|flac|mp3)\b`: "acodec:$1",
		`(?i)\b(mkv|mp4|avi)\b`: "container:$1",
	}
	
	for pattern, tagTemplate := range qualityPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(name); len(matches) > 1 {
			tag := strings.ToLower(tagTemplate)
			for i, match := range matches[1:] {
				tag = strings.Replace(tag, fmt.Sprintf("$%d", i+1), strings.ToLower(match), -1)
			}
			tags = append(tags, tag)
		}
	}
	
	// Year pattern
	yearRe := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	if matches := yearRe.FindStringSubmatch(name); len(matches) > 1 {
		tags = append(tags, "year:"+matches[1])
	}
	
	// Common tags
	commonTags := []struct {
		pattern string
		tag     string
	}{
		{`(?i)\b(directors?[.\s-]?cut)\b`, "directors-cut"},
		{`(?i)\b(extended|uncut|unrated)\b`, "extended"},
		{`(?i)\b(remaster|restored)\b`, "remastered"},
		{`(?i)\b(hdtv)\b`, "source:hdtv"},
		{`(?i)\b(web)\b`, "source:web"},
		{`(?i)\b(retail)\b`, "source:retail"},
	}
	
	for _, ct := range commonTags {
		if matched, _ := regexp.MatchString(ct.pattern, name); matched {
			tags = append(tags, ct.tag)
		}
	}
	
	return tags
}

// Helper functions

func (at *AutoTagger) isMediaFile(ext string) bool {
	mediaExts := map[string]bool{
		"mp4": true, "mkv": true, "avi": true, "mov": true,
		"webm": true, "flv": true, "wmv": true, "m4v": true,
		"mp3": true, "flac": true, "opus": true, "ogg": true,
		"m4a": true, "wav": true, "ape": true, "wma": true,
	}
	return mediaExts[ext]
}

func (at *AutoTagger) classifyResolution(width, height int) string {
	// Common resolutions
	if height >= 2160 {
		return "4k"
	} else if height >= 1440 {
		return "1440p"
	} else if height >= 1080 {
		return "1080p"
	} else if height >= 720 {
		return "720p"
	} else if height >= 480 {
		return "480p"
	}
	return ""
}

func (at *AutoTagger) calculateAspectRatio(width, height int) string {
	if height == 0 {
		return ""
	}
	
	ratio := float64(width) / float64(height)
	
	// Common aspect ratios
	if ratio > 2.35 && ratio < 2.40 {
		return "21:9"
	} else if ratio > 1.77 && ratio < 1.78 {
		return "16:9"
	} else if ratio > 1.33 && ratio < 1.34 {
		return "4:3"
	} else if ratio > 1.60 && ratio < 1.61 {
		return "16:10"
	}
	
	return ""
}

func (at *AutoTagger) parseFrameRate(frameRate string) int {
	// Format: "30/1" or "30000/1001"
	parts := strings.Split(frameRate, "/")
	if len(parts) != 2 {
		return 0
	}
	
	num, err1 := strconv.Atoi(parts[0])
	den, err2 := strconv.Atoi(parts[1])
	
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	
	fps := num / den
	
	// Common frame rates
	switch {
	case fps >= 23 && fps <= 25:
		return 24
	case fps >= 29 && fps <= 31:
		return 30
	case fps >= 47 && fps <= 51:
		return 50
	case fps >= 59 && fps <= 61:
		return 60
	default:
		return fps
	}
}

func (at *AutoTagger) extractYear(value string) int {
	// Try direct parse
	if year, err := strconv.Atoi(value); err == nil && year > 1900 && year < 2100 {
		return year
	}
	
	// Try to extract from date string
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t.Year()
	}
	
	// Try year pattern
	yearRe := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	if matches := yearRe.FindStringSubmatch(value); len(matches) > 1 {
		if year, err := strconv.Atoi(matches[1]); err == nil {
			return year
		}
	}
	
	return 0
}

func deduplicateTags(tags []string) []string {
	seen := make(map[string]bool)
	unique := []string{}
	
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if !seen[normalized] {
			seen[normalized] = true
			unique = append(unique, tag)
		}
	}
	
	return unique
}