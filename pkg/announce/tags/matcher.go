package tags

import (
	"strings"
)

// MatchMode defines how tags should be matched
type MatchMode int

const (
	// MatchAny matches if any of the query tags are present
	MatchAny MatchMode = iota
	// MatchAll matches only if all query tags are present
	MatchAll
	// MatchExact matches only the exact set of tags
	MatchExact
)

// Matcher handles tag matching operations
type Matcher struct {
	parser *TagParser
	mode   MatchMode
}

// NewMatcher creates a new tag matcher
func NewMatcher(mode MatchMode) *Matcher {
	return &Matcher{
		parser: NewTagParser(),
		mode:   mode,
	}
}

// SetMode changes the matching mode
func (m *Matcher) SetMode(mode MatchMode) {
	m.mode = mode
}

// Match checks if content tags match the query tags
func (m *Matcher) Match(contentTags, queryTags []string) (bool, error) {
	// Parse all tags
	content, err := m.parser.ParseMultiple(contentTags)
	if err != nil {
		return false, err
	}
	
	query, err := m.parser.ParseMultiple(queryTags)
	if err != nil {
		return false, err
	}
	
	switch m.mode {
	case MatchAny:
		return m.matchAny(content, query), nil
	case MatchAll:
		return m.matchAll(content, query), nil
	case MatchExact:
		return m.matchExact(content, query), nil
	default:
		return false, nil
	}
}

// MatchWithScore returns a match score between 0 and 1
func (m *Matcher) MatchWithScore(contentTags, queryTags []string) (float64, error) {
	content, err := m.parser.ParseMultiple(contentTags)
	if err != nil {
		return 0, err
	}
	
	query, err := m.parser.ParseMultiple(queryTags)
	if err != nil {
		return 0, err
	}
	
	if len(query) == 0 {
		return 0, nil
	}
	
	matches := 0
	for _, qTag := range query {
		for _, cTag := range content {
			if m.tagsMatch(qTag, cTag) {
				matches++
				break
			}
		}
	}
	
	return float64(matches) / float64(len(query)), nil
}

// FilterByTags filters content based on tag queries
func (m *Matcher) FilterByTags(items []TaggedItem, queryTags []string) ([]TaggedItem, error) {
	if len(queryTags) == 0 {
		return items, nil
	}
	
	filtered := []TaggedItem{}
	for _, item := range items {
		match, err := m.Match(item.GetTags(), queryTags)
		if err != nil {
			return nil, err
		}
		if match {
			filtered = append(filtered, item)
		}
	}
	
	return filtered, nil
}

// RankByTags ranks items by tag match score
func (m *Matcher) RankByTags(items []TaggedItem, queryTags []string) ([]ScoredItem, error) {
	scored := make([]ScoredItem, 0, len(items))
	
	for _, item := range items {
		score, err := m.MatchWithScore(item.GetTags(), queryTags)
		if err != nil {
			return nil, err
		}
		scored = append(scored, ScoredItem{
			Item:  item,
			Score: score,
		})
	}
	
	// Sort by score descending
	sortByScore(scored)
	
	return scored, nil
}

// Helper methods

func (m *Matcher) matchAny(content, query []*Tag) bool {
	for _, qTag := range query {
		for _, cTag := range content {
			if m.tagsMatch(qTag, cTag) {
				return true
			}
		}
	}
	return false
}

func (m *Matcher) matchAll(content, query []*Tag) bool {
	for _, qTag := range query {
		found := false
		for _, cTag := range content {
			if m.tagsMatch(qTag, cTag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (m *Matcher) matchExact(content, query []*Tag) bool {
	if len(content) != len(query) {
		return false
	}
	
	// Check each query tag exists in content
	for _, qTag := range query {
		found := false
		for _, cTag := range content {
			if m.tagsMatch(qTag, cTag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check each content tag exists in query
	for _, cTag := range content {
		found := false
		for _, qTag := range query {
			if m.tagsMatch(qTag, cTag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	return true
}

func (m *Matcher) tagsMatch(a, b *Tag) bool {
	// Normalize for comparison
	aNorm := normalizeTag(a.Format())
	bNorm := normalizeTag(b.Format())
	
	// Direct match
	if aNorm == bNorm {
		return true
	}
	
	// If one is namespaced and other isn't, check value match
	if a.IsNamespaced() != b.IsNamespaced() {
		aVal := normalizeTag(a.Value)
		bVal := normalizeTag(b.Value)
		return aVal == bVal
	}
	
	// Namespace wildcard matching (e.g., "res:*" matches any resolution)
	if a.IsNamespaced() && b.IsNamespaced() && a.Namespace == b.Namespace {
		if a.Value == "*" || b.Value == "*" {
			return true
		}
	}
	
	return false
}

func normalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// TaggedItem interface for items that have tags
type TaggedItem interface {
	GetTags() []string
}

// ScoredItem wraps an item with its match score
type ScoredItem struct {
	Item  TaggedItem
	Score float64
}

// sortByScore sorts items by score in descending order
func sortByScore(items []ScoredItem) {
	// Simple bubble sort for now (can be optimized if needed)
	n := len(items)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if items[j].Score < items[j+1].Score {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}
}

// ExpandQuery expands a query to include related tags
func ExpandQuery(queryTags []string) []string {
	expanded := make([]string, 0, len(queryTags)*2)
	seen := make(map[string]bool)
	
	// Add original tags
	for _, tag := range queryTags {
		normalized := normalizeTag(tag)
		if !seen[normalized] {
			expanded = append(expanded, tag)
			seen[normalized] = true
		}
	}
	
	// Add related tags
	for _, tag := range queryTags {
		related := getRelatedTags(tag)
		for _, rel := range related {
			normalized := normalizeTag(rel)
			if !seen[normalized] {
				expanded = append(expanded, rel)
				seen[normalized] = true
			}
		}
	}
	
	return expanded
}

// getRelatedTags returns tags related to the given tag
func getRelatedTags(tag string) []string {
	related := []string{}
	
	// Parse tag
	parser := NewTagParser()
	parsed, err := parser.Parse(tag)
	if err != nil {
		return related
	}
	
	// Resolution relationships
	if parsed.Namespace == "res" {
		switch parsed.Value {
		case "4k":
			related = append(related, "res:2160p", "res:uhd")
		case "1080p":
			related = append(related, "res:fhd")
		case "720p":
			related = append(related, "res:hd")
		}
	}
	
	// Codec relationships
	if parsed.Namespace == "vcodec" {
		switch parsed.Value {
		case "h265":
			related = append(related, "vcodec:hevc")
		case "hevc":
			related = append(related, "vcodec:h265")
		}
	}
	
	// Quality relationships
	if parsed.Namespace == "quality" {
		switch parsed.Value {
		case "bluray":
			related = append(related, "quality:bdrip")
		case "web":
			related = append(related, "quality:webdl", "quality:webrip")
		}
	}
	
	// Language relationships
	if parsed.Namespace == "lang" {
		// Add common language codes
		switch parsed.Value {
		case "english":
			related = append(related, "lang:en")
		case "spanish":
			related = append(related, "lang:es")
		case "french":
			related = append(related, "lang:fr")
		}
	}
	
	return related
}