package tags

// StandardNamespaces defines the standard tag namespaces and their purposes
var StandardNamespaces = map[string]string{
	// Media properties
	"res":      "Resolution (e.g., 4k, 1080p, 720p)",
	"fps":      "Frames per second (e.g., 24, 30, 60)",
	"aspect":   "Aspect ratio (e.g., 16:9, 4:3, 21:9)",
	"vcodec":   "Video codec (e.g., h264, h265, av1)",
	"acodec":   "Audio codec (e.g., aac, mp3, flac)",
	"container": "Container format (e.g., mp4, mkv, avi)",
	
	// Quality indicators
	"quality":  "Source quality (e.g., bluray, webdl, hdtv)",
	"bitrate":  "Bitrate in kbps or 'lossless'",
	"audio":    "Audio configuration (e.g., stereo, 5.1, 7.1)",
	
	// Content metadata
	"year":     "Release year or decade (e.g., 2023, 1990s)",
	"decade":   "Release decade (e.g., 1980s, 2010s)",
	"genre":    "Content genre (e.g., scifi, drama, documentary)",
	"lang":     "Language code (e.g., en, es, fr)",
	"subs":     "Subtitle languages (e.g., en, multi)",
	
	// File properties
	"size":     "File size class (e.g., tiny, small, medium, large)",
	"duration": "Duration class (e.g., short, medium, long)",
	"pages":    "Number of pages (for documents)",
	"ext":      "File extension (e.g., mp4, pdf, epub)",
	"type":     "Content type (e.g., video, audio, document)",
	
	// Source information
	"source":   "Content source (e.g., retail, web, hdtv)",
	"format":   "File format (e.g., pdf, epub, cbr)",
	
	// Special flags
	"remastered": "Content has been remastered",
	"extended":   "Extended or director's cut",
	"lossless":   "Lossless quality",
	"vector":     "Vector graphics",
	"ebook":      "Electronic book",
	"plaintext":  "Plain text content",
}

// CommonTags defines tags without namespaces that are commonly used
var CommonTags = []string{
	// Quality/version indicators
	"remastered",
	"directors-cut",
	"extended",
	"uncut",
	"unrated",
	"restored",
	
	// Format indicators
	"lossless",
	"lossy",
	"compressed",
	"vector",
	"raster",
	
	// Content indicators
	"ebook",
	"audiobook",
	"soundtrack",
	"commentary",
	"extras",
	"bonus",
}

// RecommendedTagPatterns provides examples of recommended tag usage
var RecommendedTagPatterns = map[string][]string{
	"video": {
		"res:1080p",
		"fps:24",
		"vcodec:h264",
		"acodec:aac",
		"audio:5.1",
		"quality:bluray",
		"year:2023",
		"genre:scifi",
		"lang:en",
		"subs:multi",
	},
	"audio": {
		"acodec:flac",
		"bitrate:lossless",
		"audio:stereo",
		"year:2020",
		"genre:jazz",
		"remastered",
	},
	"document": {
		"format:pdf",
		"pages:250",
		"year:2019",
		"lang:en",
		"ebook",
	},
	"image": {
		"format:png",
		"res:4k",
		"vector",
		"year:2022",
	},
}

// GetConvention returns the convention description for a namespace
func GetConvention(namespace string) (string, bool) {
	desc, ok := StandardNamespaces[namespace]
	return desc, ok
}

// IsStandardNamespace checks if a namespace is standard
func IsStandardNamespace(namespace string) bool {
	_, ok := StandardNamespaces[namespace]
	return ok
}

// IsCommonTag checks if a tag is a recognized common tag
func IsCommonTag(tag string) bool {
	for _, common := range CommonTags {
		if tag == common {
			return true
		}
	}
	return false
}

// SuggestTags suggests relevant tags based on content type
func SuggestTags(contentType string) []string {
	if patterns, ok := RecommendedTagPatterns[contentType]; ok {
		return patterns
	}
	return []string{}
}

// ValidateTagConvention checks if a tag follows conventions
func ValidateTagConvention(tag string) (valid bool, suggestion string) {
	parsed, err := NewTagParser().Parse(tag)
	if err != nil {
		return false, "Invalid tag format"
	}
	
	// Check namespace convention
	if parsed.IsNamespaced() {
		if !IsStandardNamespace(parsed.Namespace) {
			// Suggest similar namespace
			similar := findSimilarNamespace(parsed.Namespace)
			if similar != "" {
				return false, "Unknown namespace. Did you mean '" + similar + "'?"
			}
			return false, "Unknown namespace '" + parsed.Namespace + "'"
		}
	} else {
		// Check if it's a common tag
		if !IsCommonTag(parsed.Value) {
			// Suggest namespaced version if applicable
			if ns := suggestNamespace(parsed.Value); ns != "" {
				return false, "Consider using '" + ns + ":" + parsed.Value + "'"
			}
		}
	}
	
	return true, ""
}

// Helper functions

func findSimilarNamespace(ns string) string {
	// Simple similarity check
	candidates := []string{}
	for standard := range StandardNamespaces {
		if len(ns) > 0 && len(standard) > 0 && ns[0] == standard[0] {
			candidates = append(candidates, standard)
		}
	}
	
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func suggestNamespace(value string) string {
	// Suggest namespace based on common patterns
	switch value {
	case "720p", "1080p", "4k", "8k":
		return "res"
	case "24", "30", "60":
		return "fps"
	case "h264", "h265", "av1":
		return "vcodec"
	case "aac", "mp3", "flac":
		return "acodec"
	case "mp4", "mkv", "avi":
		return "container"
	case "bluray", "webdl", "hdtv":
		return "quality"
	case "en", "es", "fr", "de", "ja":
		return "lang"
	case "scifi", "drama", "comedy", "horror":
		return "genre"
	}
	return ""
}