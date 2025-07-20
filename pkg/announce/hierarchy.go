package announce

import (
	"fmt"
	"strings"
	"sync"
)

// TopicHierarchy provides comprehensive hierarchical topic organization for NoiseFS content discovery.
//
// The TopicHierarchy system implements a tree-based topic organization that enables
// structured content categorization, hierarchical browsing, and intelligent content
// discovery. It supports multi-level topic structures with metadata annotation,
// efficient lookups, and relationship-based content exploration.
//
// Key Features:
//   - Tree-based hierarchical organization with unlimited depth
//   - Fast topic lookup and navigation using path-based indexing
//   - Metadata annotation for rich topic information
//   - Relationship discovery (ancestors, descendants, siblings, cousins)
//   - Pattern-based topic search with case-insensitive matching
//   - Hash-based topic identification for privacy-preserving organization
//
// Topic Structure:
//   - Root-based tree with unique paths (e.g., "media/movies/scifi")
//   - Each topic has a SHA-256 hash for anonymous organization
//   - Parent-child relationships enable hierarchical navigation
//   - Flexible metadata system for custom topic properties
//
// Privacy Features:
//   - Topic hashes prevent enumeration while enabling exact matching
//   - Path-based organization maintains logical structure
//   - No sensitive metadata exposure beyond user-defined annotations
//
// Thread Safety: TopicHierarchy is safe for concurrent use across multiple goroutines.
// All operations use read-write mutexes for optimal concurrent performance.
type TopicHierarchy struct {
	// root is the root node of the topic hierarchy tree.
	// All topic paths are relative to this root node.
	root     *TopicNode
	
	// nodeMap provides O(1) topic lookup by topic path.
	// Maps normalized topic paths to their corresponding TopicNode instances.
	nodeMap  map[string]*TopicNode // Quick lookup by topic path
	
	// mu provides read-write mutex protection for thread-safe hierarchy operations.
	// Uses RWMutex to optimize concurrent read-heavy topic lookup workloads.
	mu       sync.RWMutex
}

// TopicNode represents a single node in the hierarchical topic organization tree.
//
// TopicNode encapsulates all information about a specific topic including its
// position in the hierarchy, identification data, relationships, and custom
// metadata. Each node maintains bidirectional relationships with its parent
// and children to enable efficient navigation in all directions.
//
// Node Structure:
//   - Unique name within its parent's children
//   - Full path for global identification
//   - SHA-256 hash for privacy-preserving topic matching
//   - Parent reference for upward navigation
//   - Children map for downward navigation
//   - Flexible metadata system for custom properties
type TopicNode struct {
	// Name is the local name of this topic within its parent.
	// Must be unique among siblings in the same parent node.
	Name        string
	
	// Path is the complete hierarchical path from root to this topic.
	// Uses "/" as separator (e.g., "media/movies/scifi").
	// Empty string indicates root node.
	Path        string              // Full path (e.g., "media/movies/scifi")
	
	// Hash is the SHA-256 hash of the normalized topic path.
	// Provides privacy-preserving topic identification and matching.
	Hash        string              // SHA-256 hash of path
	
	// Parent references the parent node in the hierarchy.
	// Nil for the root node, non-nil for all other nodes.
	Parent      *TopicNode
	
	// Children maps child topic names to their TopicNode instances.
	// Enables efficient child lookup and iteration.
	Children    map[string]*TopicNode
	
	// Metadata stores custom key-value properties for this topic.
	// Enables flexible topic annotation and categorization.
	Metadata    map[string]string   // Optional metadata
}

// NewTopicHierarchy creates a new hierarchical topic organization system with an initialized root node.
//
// This constructor establishes a complete topic hierarchy with a root node and
// supporting data structures for efficient topic management. The hierarchy
// starts empty except for the root node and can be populated with topics
// using AddTopic or initialized with common topics using BuildCommonHierarchy.
//
// Returns:
//   A new TopicHierarchy ready for topic organization and content discovery
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, grows with added topics
//
// Initial State:
//   - Root node with empty path and computed hash
//   - Empty nodeMap for topic lookup
//   - Thread-safe concurrent access support
//
// Example:
//   hierarchy := announce.NewTopicHierarchy()
//   node, err := hierarchy.AddTopic("media/movies/scifi", metadata)
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

// AddTopic creates or updates a topic node in the hierarchy with automatic path construction.
//
// This method adds a complete topic path to the hierarchy, creating all necessary
// intermediate nodes along the path. If the topic already exists, it updates
// the metadata without modifying the existing structure. The operation is atomic
// and creates a complete path from root to the specified topic.
//
// Parameters:
//   - topicPath: Hierarchical topic path using "/" separators (e.g., "media/movies/scifi")
//   - metadata: Optional key-value metadata to associate with the topic
//
// Returns:
//   - TopicNode for the created or updated topic
//   - error if topic creation fails (currently never fails)
//
// Time Complexity: O(d) where d is the depth of the topic path
// Space Complexity: O(d) for path construction and node creation
//
// Path Handling:
//   - Normalizes topic paths for consistent handling
//   - Creates intermediate nodes automatically if they don't exist
//   - Empty path returns the root node
//   - Handles duplicate separators and trailing slashes
//
// Metadata Handling:
//   - Adds new metadata to existing topics without overwriting
//   - Updates existing metadata keys with new values
//   - Preserves existing metadata not specified in the update
//
// Thread Safety: Uses write lock for exclusive access during topic creation.
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

// GetTopic retrieves a topic node by its hierarchical path with efficient O(1) lookup.
//
// This method provides fast topic retrieval using path-based indexing. It
// normalizes the input path and performs a direct lookup in the nodeMap
// for optimal performance. The method is read-only and safe for concurrent use.
//
// Parameters:
//   - topicPath: Hierarchical topic path to retrieve (e.g., "media/movies/scifi")
//
// Returns:
//   - TopicNode for the specified path if found
//   - bool indicating whether the topic exists in the hierarchy
//
// Time Complexity: O(1) for direct map lookup
// Space Complexity: O(1)
//
// Path Handling:
//   - Normalizes topic paths for consistent lookup
//   - Empty path returns the root node
//   - Case-sensitive path matching
//
// Thread Safety: Uses read lock for safe concurrent access.
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

// GetChildren retrieves all direct child topics of the specified parent topic.
//
// This method returns the immediate children of a topic without recursing into
// deeper levels of the hierarchy. It provides efficient access to the next
// level of topic organization for navigation and discovery interfaces.
//
// Parameters:
//   - topicPath: Hierarchical path of the parent topic
//
// Returns:
//   - Slice of direct child TopicNode instances
//   - error if the parent topic is not found
//
// Time Complexity: O(c) where c is the number of direct children
// Space Complexity: O(c) for the result slice
//
// Use Cases:
//   - Navigation interface showing topic hierarchy levels
//   - Breadcrumb navigation systems
//   - Topic browsing and exploration
//   - Content organization displays
//
// Thread Safety: Uses read lock for safe concurrent access.
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

// GetDescendants retrieves all descendant topics in the subtree rooted at the specified topic.
//
// This method performs a recursive traversal to collect all topics at any depth
// below the specified parent topic. It provides comprehensive access to entire
// topic subtrees for batch operations, full subtree searches, and hierarchical
// content discovery across all levels.
//
// Parameters:
//   - topicPath: Hierarchical path of the root topic for subtree traversal
//
// Returns:
//   - Slice of all descendant TopicNode instances in the subtree
//   - error if the root topic is not found
//
// Time Complexity: O(n) where n is the total number of descendants
// Space Complexity: O(n) for the result slice and recursion stack
//
// Traversal Strategy:
//   - Depth-first recursive collection of all descendant nodes
//   - Includes topics at all levels below the specified root
//   - Maintains original hierarchy relationships in results
//
// Use Cases:
//   - Bulk operations on entire topic subtrees
//   - Content discovery across topic hierarchies
//   - Topic migration and reorganization
//   - Comprehensive topic analysis and statistics
//
// Thread Safety: Uses read lock for safe concurrent access.
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

// GetAncestors retrieves all ancestor topics from the specified topic up to the root.
//
// This method traverses upward through the hierarchy to collect all parent
// topics from the specified topic to the root level. It provides access to
// the complete ancestry chain for breadcrumb navigation, context analysis,
// and hierarchical relationship discovery.
//
// Parameters:
//   - topicPath: Hierarchical path of the topic for ancestor retrieval
//
// Returns:
//   - Slice of ancestor TopicNode instances ordered from immediate parent to root
//   - error if the specified topic is not found
//
// Time Complexity: O(d) where d is the depth of the topic in the hierarchy
// Space Complexity: O(d) for the result slice
//
// Traversal Order:
//   - Starts from the immediate parent of the specified topic
//   - Continues upward until reaching the root node
//   - Excludes the root node from the result (stops before root)
//   - Returns empty slice for root-level topics
//
// Use Cases:
//   - Breadcrumb navigation generation
//   - Context-aware topic display
//   - Hierarchical path analysis
//   - Parent-child relationship exploration
//
// Thread Safety: Uses read lock for safe concurrent access.
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

// GetRelated discovers topics related to the specified topic through hierarchical relationships.
//
// This method finds related topics by exploring sibling and cousin relationships
// within the topic hierarchy. It provides intelligent content discovery by
// identifying topics that share common parent nodes or are structurally
// similar within the hierarchical organization.
//
// Parameters:
//   - topicPath: Hierarchical path of the topic for relationship discovery
//   - maxDistance: Maximum relationship distance (1=siblings, 2=siblings+cousins)
//
// Returns:
//   - Slice of related TopicNode instances with shared hierarchical context
//   - error if the specified topic is not found
//
// Time Complexity: O(s + c) where s is siblings, c is cousins
// Space Complexity: O(s + c) for result collection and deduplication
//
// Relationship Types:
//   - Siblings: Topics sharing the same immediate parent
//   - Cousins: Topics sharing the same grandparent (maxDistance >= 2)
//   - Deduplication ensures each related topic appears only once
//
// Use Cases:
//   - Related content discovery and suggestions
//   - Topic recommendation systems
//   - Content exploration and navigation
//   - Hierarchical browsing enhancements
//
// Thread Safety: Uses read lock for safe concurrent access.
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

// FindTopics performs pattern-based search across all topics in the hierarchy.
//
// This method searches through all topic paths and names to find matches
// for the specified pattern. It provides flexible topic discovery through
// case-insensitive substring matching on both full paths and topic names,
// enabling users to find topics without knowing exact hierarchical paths.
//
// Parameters:
//   - pattern: Search pattern for case-insensitive substring matching
//
// Returns:
//   - Slice of TopicNode instances matching the search pattern
//
// Time Complexity: O(n*m) where n is total topics, m is pattern length
// Space Complexity: O(k) where k is the number of matching topics
//
// Search Strategy:
//   - Case-insensitive substring matching on topic paths
//   - Case-insensitive substring matching on topic names
//   - Returns topics matching either path or name criteria
//   - No duplicate results for topics matching both criteria
//
// Use Cases:
//   - Topic search and discovery interfaces
//   - Auto-completion and suggestion systems
//   - Content organization and navigation
//   - Administrative topic management
//
// Thread Safety: Uses read lock for safe concurrent access.
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

// GetTopicHashes retrieves SHA-256 hashes for a topic and optionally its entire subtree.
//
// This method provides access to topic hashes for privacy-preserving content
// organization and matching. It can return just the specified topic's hash
// or include all descendant topic hashes for comprehensive subtree operations.
//
// Parameters:
//   - topicPath: Hierarchical path of the root topic for hash retrieval
//   - includeDescendants: Whether to include hashes from all descendant topics
//
// Returns:
//   - Slice of SHA-256 topic hashes (specified topic + descendants if requested)
//   - error if the specified topic is not found
//
// Time Complexity: O(1) for single topic, O(n) for subtree where n is descendants
// Space Complexity: O(1) for single topic, O(n) for subtree hash collection
//
// Hash Properties:
//   - SHA-256 hashes provide privacy-preserving topic identification
//   - Hashes enable exact topic matching without exposing topic structure
//   - Consistent hash generation ensures reliable topic organization
//
// Use Cases:
//   - Privacy-preserving topic matching in announcements
//   - Bulk topic operations using hash-based identification
//   - Topic-based content filtering and organization
//   - Hierarchical content discovery with privacy protection
//
// Thread Safety: Calls GetTopic and GetDescendants which use appropriate locking.
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

// collectDescendants performs recursive depth-first traversal to gather all descendant topics.
//
// This helper method recursively visits all child nodes of the specified topic
// and adds them to the result slice. It maintains the hierarchical relationship
// order through depth-first traversal, ensuring systematic coverage of the
// entire subtree rooted at the given node.
//
// Parameters:
//   - node: Root node for descendant collection
//   - result: Pointer to slice for accumulating descendant nodes
//
// Time Complexity: O(n) where n is the number of descendant nodes
// Space Complexity: O(d) for recursion stack where d is maximum depth
//
// Traversal Strategy:
//   - Depth-first recursive traversal ensures systematic coverage
//   - Visits each descendant exactly once
//   - Maintains hierarchical order in result collection
func (th *TopicHierarchy) collectDescendants(node *TopicNode, result *[]*TopicNode) {
	for _, child := range node.Children {
		*result = append(*result, child)
		th.collectDescendants(child, result)
	}
}

// BuildCommonHierarchy creates a pre-populated topic hierarchy with standard content categories.
//
// This utility function builds a comprehensive topic hierarchy containing
// common content categories used in typical media and software organization.
// It provides a ready-to-use topic structure covering major content types
// and popular subcategories for immediate deployment.
//
// Returns:
//   A TopicHierarchy pre-populated with standard content organization topics
//
// Time Complexity: O(t) where t is the number of predefined topics
// Space Complexity: O(t) for the complete topic hierarchy structure
//
// Topic Categories Included:
//   - Media: movies, TV shows, music with genre subcategories
//   - Software: operating systems, tools, games with type classifications
//   - Documents: books, papers, manuals with content type organization
//   - Education: courses, tutorials, lectures with learning type structure
//
// Hierarchy Structure:
//   - 3-4 level deep organization (category/type/subtype/specific)
//   - Metadata annotations for category types and classifications
//   - Comprehensive coverage of common content organization needs
//
// Use Cases:
//   - Quick deployment with standard topic organization
//   - Template for custom topic hierarchy development
//   - Common content categorization systems
//   - Default topic structure for NoiseFS deployments
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

// GetTopicDepth calculates the hierarchical depth level of a topic path.
//
// This utility function determines how many levels deep a topic path is
// within the hierarchy by counting path separators. It provides depth
// information for navigation, organization, and hierarchy analysis.
//
// Parameters:
//   - topicPath: Hierarchical topic path to analyze (e.g., "media/movies/scifi")
//
// Returns:
//   - Integer depth level (0 for root, 1 for top-level, 2+ for deeper levels)
//
// Time Complexity: O(n) where n is the length of the topic path
// Space Complexity: O(1)
//
// Depth Calculation:
//   - Empty path returns depth 0 (root level)
//   - Single component returns depth 1 (top-level topic)
//   - Each "/" separator adds one level of depth
//
// Examples:
//   - "" -> 0 (root)
//   - "media" -> 1 (top-level)
//   - "media/movies" -> 2 (second level)
//   - "media/movies/scifi" -> 3 (third level)
func GetTopicDepth(topicPath string) int {
	if topicPath == "" {
		return 0
	}
	return strings.Count(topicPath, "/") + 1
}

// GetTopicParent extracts the parent topic path from a hierarchical topic path.
//
// This utility function determines the immediate parent topic by removing
// the last path component from the specified topic path. It provides
// efficient parent path calculation for hierarchy navigation and relationship
// analysis without requiring full hierarchy traversal.
//
// Parameters:
//   - topicPath: Hierarchical topic path to analyze (e.g., "media/movies/scifi")
//
// Returns:
//   - Parent topic path with the last component removed (e.g., "media/movies")
//   - Empty string if the topic is at root level or has no parent
//
// Time Complexity: O(n) where n is the length of the topic path
// Space Complexity: O(1)
//
// Parent Extraction:
//   - Finds the last "/" separator in the path
//   - Returns everything before the last separator
//   - Returns empty string if no separator found (root-level topic)
//
// Examples:
//   - "media/movies/scifi" -> "media/movies"
//   - "media/movies" -> "media"
//   - "media" -> "" (root parent)
//   - "" -> "" (root has no parent)
func GetTopicParent(topicPath string) string {
	lastSlash := strings.LastIndex(topicPath, "/")
	if lastSlash < 0 {
		return ""
	}
	return topicPath[:lastSlash]
}

// IsSubtopic determines whether one topic is hierarchically contained within another topic.
//
// This utility function performs hierarchical relationship analysis by checking
// if the subtopic path starts with the parent path followed by a separator.
// It provides efficient subtopic relationship validation without requiring
// full hierarchy traversal or topic node access.
//
// Parameters:
//   - subtopic: Topic path to test for containment (e.g., "media/movies/scifi")
//   - parent: Parent topic path to test against (e.g., "media/movies")
//
// Returns:
//   - true if subtopic is hierarchically contained within parent
//   - false if subtopic is not a descendant of parent
//
// Time Complexity: O(min(n, m)) where n and m are the path lengths
// Space Complexity: O(1)
//
// Relationship Testing:
//   - Checks if subtopic starts with parent path + "/"
//   - Ensures exact hierarchical containment (not just prefix matching)
//   - Prevents false positives from similar path prefixes
//
// Examples:
//   - IsSubtopic("media/movies/scifi", "media/movies") -> true
//   - IsSubtopic("media/movies", "media") -> true
//   - IsSubtopic("media/music", "media/movies") -> false
//   - IsSubtopic("media", "media") -> false (same topic, not subtopic)
func IsSubtopic(subtopic, parent string) bool {
	return strings.HasPrefix(subtopic, parent+"/")
}