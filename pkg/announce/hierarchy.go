package announce

import (
	"fmt"
	"strings"
	"sync"
)

// TopicHierarchy manages hierarchical topic relationships
type TopicHierarchy struct {
	root     *TopicNode
	nodeMap  map[string]*TopicNode // Quick lookup by topic path
	mu       sync.RWMutex
}

// TopicNode represents a node in the topic hierarchy
type TopicNode struct {
	Name        string
	Path        string              // Full path (e.g., "media/movies/scifi")
	Hash        string              // SHA-256 hash of path
	Parent      *TopicNode
	Children    map[string]*TopicNode
	Metadata    map[string]string   // Optional metadata
}

// NewTopicHierarchy creates a new topic hierarchy
func NewTopicHierarchy() *TopicHierarchy {
	root := &TopicNode{
		Name:     "root",
		Path:     "",
		Hash:     HashTopic(""),
		Children: make(map[string]*TopicNode),
		Metadata: make(map[string]string),
	}
	
	return &TopicHierarchy{
		root:    root,
		nodeMap: make(map[string]*TopicNode),
	}
}

// AddTopic adds a topic to the hierarchy
func (th *TopicHierarchy) AddTopic(topicPath string, metadata map[string]string) (*TopicNode, error) {
	th.mu.Lock()
	defer th.mu.Unlock()
	
	// Normalize path
	topicPath = normalizeTopic(topicPath)
	if topicPath == "" {
		return th.root, nil
	}
	
	// Check if already exists
	if node, exists := th.nodeMap[topicPath]; exists {
		// Update metadata if provided
		if metadata != nil {
			for k, v := range metadata {
				node.Metadata[k] = v
			}
		}
		return node, nil
	}
	
	// Split path into components
	parts := strings.Split(topicPath, "/")
	
	// Build path from root
	current := th.root
	currentPath := ""
	
	for _, part := range parts {
		if part == "" {
			continue
		}
		
		// Build path
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath = currentPath + "/" + part
		}
		
		// Check if child exists
		child, exists := current.Children[part]
		if !exists {
			// Create new node
			child = &TopicNode{
				Name:     part,
				Path:     currentPath,
				Hash:     HashTopic(currentPath),
				Parent:   current,
				Children: make(map[string]*TopicNode),
				Metadata: make(map[string]string),
			}
			current.Children[part] = child
			th.nodeMap[currentPath] = child
		}
		
		current = child
	}
	
	// Add metadata to final node
	if metadata != nil {
		for k, v := range metadata {
			current.Metadata[k] = v
		}
	}
	
	return current, nil
}

// GetTopic retrieves a topic node by path
func (th *TopicHierarchy) GetTopic(topicPath string) (*TopicNode, bool) {
	th.mu.RLock()
	defer th.mu.RUnlock()
	
	topicPath = normalizeTopic(topicPath)
	if topicPath == "" {
		return th.root, true
	}
	
	node, exists := th.nodeMap[topicPath]
	return node, exists
}

// GetChildren returns all direct children of a topic
func (th *TopicHierarchy) GetChildren(topicPath string) ([]*TopicNode, error) {
	th.mu.RLock()
	defer th.mu.RUnlock()
	
	node, exists := th.GetTopic(topicPath)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", topicPath)
	}
	
	children := make([]*TopicNode, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child)
	}
	
	return children, nil
}

// GetDescendants returns all descendants of a topic (recursive)
func (th *TopicHierarchy) GetDescendants(topicPath string) ([]*TopicNode, error) {
	th.mu.RLock()
	defer th.mu.RUnlock()
	
	node, exists := th.GetTopic(topicPath)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", topicPath)
	}
	
	descendants := []*TopicNode{}
	th.collectDescendants(node, &descendants)
	
	return descendants, nil
}

// GetAncestors returns all ancestors of a topic (up to root)
func (th *TopicHierarchy) GetAncestors(topicPath string) ([]*TopicNode, error) {
	th.mu.RLock()
	defer th.mu.RUnlock()
	
	node, exists := th.GetTopic(topicPath)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", topicPath)
	}
	
	ancestors := []*TopicNode{}
	current := node.Parent
	
	for current != nil && current != th.root {
		ancestors = append(ancestors, current)
		current = current.Parent
	}
	
	return ancestors, nil
}

// GetRelated returns related topics (siblings and cousins)
func (th *TopicHierarchy) GetRelated(topicPath string, maxDistance int) ([]*TopicNode, error) {
	th.mu.RLock()
	defer th.mu.RUnlock()
	
	node, exists := th.GetTopic(topicPath)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", topicPath)
	}
	
	related := []*TopicNode{}
	visited := make(map[string]bool)
	
	// Get siblings (same parent)
	if node.Parent != nil {
		for _, sibling := range node.Parent.Children {
			if sibling != node && !visited[sibling.Path] {
				related = append(related, sibling)
				visited[sibling.Path] = true
			}
		}
	}
	
	// Get cousins (children of parent's siblings)
	if maxDistance > 1 && node.Parent != nil && node.Parent.Parent != nil {
		for _, uncle := range node.Parent.Parent.Children {
			if uncle != node.Parent {
				for _, cousin := range uncle.Children {
					if !visited[cousin.Path] {
						related = append(related, cousin)
						visited[cousin.Path] = true
					}
				}
			}
		}
	}
	
	return related, nil
}

// FindTopics searches for topics matching a pattern
func (th *TopicHierarchy) FindTopics(pattern string) []*TopicNode {
	th.mu.RLock()
	defer th.mu.RUnlock()
	
	pattern = strings.ToLower(pattern)
	matches := []*TopicNode{}
	
	for path, node := range th.nodeMap {
		if strings.Contains(strings.ToLower(path), pattern) ||
		   strings.Contains(strings.ToLower(node.Name), pattern) {
			matches = append(matches, node)
		}
	}
	
	return matches
}

// GetTopicHashes returns all topic hashes in a subtree
func (th *TopicHierarchy) GetTopicHashes(topicPath string, includeDescendants bool) ([]string, error) {
	node, exists := th.GetTopic(topicPath)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", topicPath)
	}
	
	hashes := []string{node.Hash}
	
	if includeDescendants {
		descendants, _ := th.GetDescendants(topicPath)
		for _, desc := range descendants {
			hashes = append(hashes, desc.Hash)
		}
	}
	
	return hashes, nil
}

// Helper methods

func (th *TopicHierarchy) collectDescendants(node *TopicNode, result *[]*TopicNode) {
	for _, child := range node.Children {
		*result = append(*result, child)
		th.collectDescendants(child, result)
	}
}

// BuildCommonHierarchy builds a common topic hierarchy
func BuildCommonHierarchy() *TopicHierarchy {
	th := NewTopicHierarchy()
	
	// Media hierarchy
	commonTopics := []struct {
		path     string
		metadata map[string]string
	}{
		// Media
		{"media", map[string]string{"type": "category"}},
		{"media/movies", map[string]string{"type": "subcategory"}},
		{"media/movies/action", nil},
		{"media/movies/comedy", nil},
		{"media/movies/drama", nil},
		{"media/movies/horror", nil},
		{"media/movies/scifi", nil},
		{"media/movies/documentary", nil},
		
		{"media/tv", map[string]string{"type": "subcategory"}},
		{"media/tv/drama", nil},
		{"media/tv/comedy", nil},
		{"media/tv/reality", nil},
		{"media/tv/documentary", nil},
		{"media/tv/anime", nil},
		
		{"media/music", map[string]string{"type": "subcategory"}},
		{"media/music/rock", nil},
		{"media/music/pop", nil},
		{"media/music/jazz", nil},
		{"media/music/classical", nil},
		{"media/music/electronic", nil},
		{"media/music/hiphop", nil},
		
		// Software
		{"software", map[string]string{"type": "category"}},
		{"software/os", map[string]string{"type": "subcategory"}},
		{"software/os/linux", nil},
		{"software/os/windows", nil},
		{"software/os/macos", nil},
		
		{"software/tools", map[string]string{"type": "subcategory"}},
		{"software/games", map[string]string{"type": "subcategory"}},
		
		// Documents
		{"documents", map[string]string{"type": "category"}},
		{"documents/books", map[string]string{"type": "subcategory"}},
		{"documents/books/fiction", nil},
		{"documents/books/nonfiction", nil},
		{"documents/books/technical", nil},
		
		{"documents/papers", map[string]string{"type": "subcategory"}},
		{"documents/manuals", map[string]string{"type": "subcategory"}},
		
		// Educational
		{"education", map[string]string{"type": "category"}},
		{"education/courses", map[string]string{"type": "subcategory"}},
		{"education/tutorials", map[string]string{"type": "subcategory"}},
		{"education/lectures", map[string]string{"type": "subcategory"}},
	}
	
	for _, topic := range commonTopics {
		th.AddTopic(topic.path, topic.metadata)
	}
	
	return th
}

// TopicPath utilities

// GetTopicDepth returns the depth of a topic path
func GetTopicDepth(topicPath string) int {
	if topicPath == "" {
		return 0
	}
	return strings.Count(topicPath, "/") + 1
}

// GetTopicParent returns the parent path of a topic
func GetTopicParent(topicPath string) string {
	lastSlash := strings.LastIndex(topicPath, "/")
	if lastSlash < 0 {
		return ""
	}
	return topicPath[:lastSlash]
}

// IsSubtopic checks if one topic is a subtopic of another
func IsSubtopic(subtopic, parent string) bool {
	return strings.HasPrefix(subtopic, parent+"/")
}