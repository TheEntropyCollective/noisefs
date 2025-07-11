package tags

import (
	"testing"
)

type testItem struct {
	tags []string
}

func (t testItem) GetTags() []string {
	return t.tags
}

func TestMatcherModes(t *testing.T) {
	tests := []struct {
		name        string
		mode        MatchMode
		contentTags []string
		queryTags   []string
		expected    bool
	}{
		{
			name:        "MatchAny - should match with one common tag",
			mode:        MatchAny,
			contentTags: []string{"res:4k", "genre:scifi", "year:2023"},
			queryTags:   []string{"res:4k", "genre:drama"},
			expected:    true,
		},
		{
			name:        "MatchAny - no common tags",
			mode:        MatchAny,
			contentTags: []string{"res:1080p", "genre:scifi"},
			queryTags:   []string{"res:4k", "genre:drama"},
			expected:    false,
		},
		{
			name:        "MatchAll - all query tags present",
			mode:        MatchAll,
			contentTags: []string{"res:4k", "genre:scifi", "year:2023", "lang:en"},
			queryTags:   []string{"res:4k", "genre:scifi"},
			expected:    true,
		},
		{
			name:        "MatchAll - missing one query tag",
			mode:        MatchAll,
			contentTags: []string{"res:4k", "genre:scifi"},
			queryTags:   []string{"res:4k", "genre:scifi", "year:2023"},
			expected:    false,
		},
		{
			name:        "MatchExact - exact same tags",
			mode:        MatchExact,
			contentTags: []string{"res:4k", "genre:scifi"},
			queryTags:   []string{"res:4k", "genre:scifi"},
			expected:    true,
		},
		{
			name:        "MatchExact - different number of tags",
			mode:        MatchExact,
			contentTags: []string{"res:4k", "genre:scifi", "year:2023"},
			queryTags:   []string{"res:4k", "genre:scifi"},
			expected:    false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMatcher(tt.mode)
			result, err := matcher.Match(tt.contentTags, tt.queryTags)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMatchWithScore(t *testing.T) {
	tests := []struct {
		name        string
		contentTags []string
		queryTags   []string
		expected    float64
	}{
		{
			name:        "All tags match",
			contentTags: []string{"res:4k", "genre:scifi", "year:2023"},
			queryTags:   []string{"res:4k", "genre:scifi"},
			expected:    1.0,
		},
		{
			name:        "Half tags match",
			contentTags: []string{"res:4k", "genre:drama"},
			queryTags:   []string{"res:4k", "genre:scifi"},
			expected:    0.5,
		},
		{
			name:        "No tags match",
			contentTags: []string{"res:1080p", "genre:drama"},
			queryTags:   []string{"res:4k", "genre:scifi"},
			expected:    0.0,
		},
		{
			name:        "Empty query tags",
			contentTags: []string{"res:4k", "genre:scifi"},
			queryTags:   []string{},
			expected:    0.0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMatcher(MatchAny)
			score, err := matcher.MatchWithScore(tt.contentTags, tt.queryTags)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if score != tt.expected {
				t.Errorf("Expected score %f, got %f", tt.expected, score)
			}
		})
	}
}

func TestWildcardMatching(t *testing.T) {
	matcher := NewMatcher(MatchAny)
	
	// Test namespace wildcard
	match, err := matcher.Match(
		[]string{"res:4k", "genre:scifi"},
		[]string{"res:*"},
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !match {
		t.Error("Expected wildcard match for res:*")
	}
}

func TestNormalizedMatching(t *testing.T) {
	matcher := NewMatcher(MatchAny)
	
	// Test case-insensitive matching
	match, err := matcher.Match(
		[]string{"genre:SciFi", "RES:4K"},
		[]string{"genre:scifi", "res:4k"},
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !match {
		t.Error("Expected case-insensitive match")
	}
}

func TestRankByTags(t *testing.T) {
	items := []TaggedItem{
		testItem{tags: []string{"res:4k", "genre:scifi", "year:2023"}},
		testItem{tags: []string{"res:1080p", "genre:scifi"}},
		testItem{tags: []string{"res:4k", "genre:drama"}},
		testItem{tags: []string{"res:720p", "genre:comedy"}},
	}
	
	matcher := NewMatcher(MatchAny)
	scored, err := matcher.RankByTags(items, []string{"res:4k", "genre:scifi"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Check that items are sorted by score
	if len(scored) != 4 {
		t.Fatalf("Expected 4 scored items, got %d", len(scored))
	}
	
	// First item should have highest score (both tags match)
	if scored[0].Score != 1.0 {
		t.Errorf("Expected first item score 1.0, got %f", scored[0].Score)
	}
	
	// Last item should have lowest score (no tags match)
	if scored[3].Score != 0.0 {
		t.Errorf("Expected last item score 0.0, got %f", scored[3].Score)
	}
}

func TestExpandQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Expand 4k resolution",
			input:    []string{"res:4k"},
			expected: []string{"res:4k", "res:2160p", "res:uhd"},
		},
		{
			name:     "Expand h265 codec",
			input:    []string{"vcodec:h265"},
			expected: []string{"vcodec:h265", "vcodec:hevc"},
		},
		{
			name:     "No expansion for unknown tags",
			input:    []string{"custom:tag"},
			expected: []string{"custom:tag"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded := ExpandQuery(tt.input)
			if len(expanded) != len(tt.expected) {
				t.Errorf("Expected %d tags, got %d", len(tt.expected), len(expanded))
				return
			}
			
			// Check all expected tags are present
			for _, exp := range tt.expected {
				found := false
				for _, tag := range expanded {
					if normalizeTag(tag) == normalizeTag(exp) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected tag %s not found in expanded query", exp)
				}
			}
		})
	}
}