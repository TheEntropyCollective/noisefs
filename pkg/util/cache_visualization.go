package util

import (
	"fmt"
	"strings"
)

// CacheVisualization provides visual representation of cache usage
type CacheVisualization struct {
	Width int // Width of the visualization bar
}

// NewCacheVisualization creates a new cache visualization
func NewCacheVisualization(width int) *CacheVisualization {
	if width <= 0 {
		width = 50 // Default width
	}
	return &CacheVisualization{Width: width}
}

// RenderCacheUsage renders a visual representation of cache usage
func (cv *CacheVisualization) RenderCacheUsage(personalPercent, altruisticPercent float64) string {
	// Calculate filled portions
	personalBlocks := int(personalPercent / 100.0 * float64(cv.Width))
	altruisticBlocks := int(altruisticPercent / 100.0 * float64(cv.Width))
	emptyBlocks := cv.Width - personalBlocks - altruisticBlocks
	
	// Ensure we don't exceed the width
	if personalBlocks+altruisticBlocks > cv.Width {
		// Scale down proportionally
		total := personalBlocks + altruisticBlocks
		personalBlocks = personalBlocks * cv.Width / total
		altruisticBlocks = cv.Width - personalBlocks
		emptyBlocks = 0
	}
	
	// Build the bar
	bar := "["
	bar += strings.Repeat("█", personalBlocks)     // Full blocks for personal
	bar += strings.Repeat("▒", altruisticBlocks)  // Medium blocks for altruistic
	bar += strings.Repeat("░", emptyBlocks)       // Light blocks for empty
	bar += "]"
	
	return bar
}

// RenderFlexPoolUsage renders the flex pool usage
func (cv *CacheVisualization) RenderFlexPoolUsage(flexUsage float64) string {
	// Calculate filled portion
	filledBlocks := int(flexUsage * float64(cv.Width))
	emptyBlocks := cv.Width - filledBlocks
	
	// Build the bar
	bar := "["
	bar += strings.Repeat("▓", filledBlocks)  // Filled flex pool
	bar += strings.Repeat("░", emptyBlocks)   // Empty flex pool
	bar += "]"
	
	return bar
}

// RenderCacheSummary renders a complete cache summary with visuals
func (cv *CacheVisualization) RenderCacheSummary(
	personalSize, altruisticSize, totalCapacity int64,
	flexPoolUsage float64,
	minPersonalCache int64,
) string {
	var output strings.Builder
	
	// Calculate percentages
	personalPercent := 0.0
	altruisticPercent := 0.0
	if totalCapacity > 0 {
		personalPercent = float64(personalSize) / float64(totalCapacity) * 100
		altruisticPercent = float64(altruisticSize) / float64(totalCapacity) * 100
	}
	
	// Header
	output.WriteString("Cache Utilization:\n")
	
	// Total usage bar
	output.WriteString(fmt.Sprintf("Total: %s %.1f%%\n", 
		cv.RenderCacheUsage(personalPercent, altruisticPercent),
		personalPercent+altruisticPercent))
	
	// Legend
	output.WriteString(fmt.Sprintf("       █ Personal (%.1f%%)  ▒ Altruistic (%.1f%%)  ░ Free (%.1f%%)\n",
		personalPercent, altruisticPercent, 100-personalPercent-altruisticPercent))
	
	// Flex pool usage
	output.WriteString(fmt.Sprintf("\nFlex Pool: %s %.1f%%\n",
		cv.RenderFlexPoolUsage(flexPoolUsage),
		flexPoolUsage*100))
	
	// Min personal cache indicator
	if totalCapacity > 0 {
		minPersonalPercent := float64(minPersonalCache) / float64(totalCapacity) * 100
		minPersonalBlocks := int(minPersonalPercent / 100.0 * float64(cv.Width))
		
		// Build indicator line
		indicator := strings.Repeat(" ", minPersonalBlocks) + "↑"
		output.WriteString(fmt.Sprintf("          %s Min Personal (%.1f%%)\n", 
			indicator, minPersonalPercent))
	}
	
	return output.String()
}