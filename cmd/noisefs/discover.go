package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/config"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/store"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// discoverCommand handles the discover subcommand
func discoverCommand(args []string, quiet bool, jsonOutput bool) error {
	flagSet := flag.NewFlagSet("discover", flag.ExitOnError)
	
	var (
		tags     = flagSet.String("tags", "", "Filter by tags (comma-separated)")
		since    = flagSet.Duration("since", 24*time.Hour, "Show announcements from this duration ago")
		limit    = flagSet.Int("limit", 50, "Maximum number of results")
		topic    = flagSet.String("topic", "", "Filter by specific topic")
		category = flagSet.String("category", "", "Filter by category (video/audio/document/etc)")
		help     = flagSet.Bool("help", false, "Show help for discover command")
	)
	
	flagSet.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: noisefs discover [options]\n\n")
		fmt.Fprintf(os.Stderr, "Discover announcements from subscribed topics.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flagSet.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  noisefs discover                          # Show recent announcements\n")
		fmt.Fprintf(os.Stderr, "  noisefs discover --tags \"4k,scifi\"        # Filter by tags\n")
		fmt.Fprintf(os.Stderr, "  noisefs discover --category video         # Show only videos\n")
		fmt.Fprintf(os.Stderr, "  noisefs discover --since 1h               # Last hour only\n")
	}
	
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	
	if *help {
		flagSet.Usage()
		return nil
	}
	
	// Load announcement store
	storeConfig := store.DefaultStoreConfig(filepath.Join(config.GetConfigDir(), "announcements"))
	annStore, err := store.NewStore(storeConfig)
	if err != nil {
		return fmt.Errorf("failed to open announcement store: %w", err)
	}
	defer annStore.Close()
	
	// Get announcements based on filters
	var announcements []*store.StoredAnnouncement
	
	if *tags != "" {
		// Search by tags
		tagList := strings.Split(*tags, ",")
		for i, tag := range tagList {
			tagList[i] = strings.TrimSpace(tag)
		}
		announcements, err = annStore.Search(tagList, *limit)
	} else if *topic != "" {
		// Get by specific topic
		topicHash := announce.HashTopic(*topic)
		announcements, err = annStore.GetByTopic(topicHash)
	} else {
		// Get recent announcements
		sinceTime := time.Now().Add(-*since)
		announcements, err = annStore.GetRecent(sinceTime, *limit)
	}
	
	if err != nil {
		return fmt.Errorf("failed to retrieve announcements: %w", err)
	}
	
	// Filter by category if specified
	if *category != "" {
		filtered := []*store.StoredAnnouncement{}
		for _, ann := range announcements {
			if ann.Category == *category {
				filtered = append(filtered, ann)
			}
		}
		announcements = filtered
	}
	
	// Load subscriptions to show topic names
	configPath := filepath.Join(config.GetConfigDir(), "subscriptions.json")
	subConfig, _ := config.LoadSubscriptions(configPath)
	
	// Create topic map
	topicMap := make(map[string]string)
	if subConfig != nil {
		for _, sub := range subConfig.GetAll() {
			topicMap[sub.TopicHash] = sub.Topic
		}
	}
	
	// Output results
	if jsonOutput {
		results := []map[string]interface{}{}
		for _, ann := range announcements {
			result := map[string]interface{}{
				"descriptor":   ann.Descriptor,
				"topic_hash":   ann.TopicHash,
				"category":     ann.Category,
				"size_class":   ann.SizeClass,
				"timestamp":    ann.Timestamp,
				"received_at":  ann.ReceivedAt,
				"source":       ann.Source,
			}
			if topic, ok := topicMap[ann.TopicHash]; ok {
				result["topic"] = topic
			}
			results = append(results, result)
		}
		util.PrintJSON(map[string]interface{}{
			"announcements": results,
			"count":         len(results),
		})
		return nil
	}
	
	// Text output
	if len(announcements) == 0 {
		fmt.Println("No announcements found")
		return nil
	}
	
	fmt.Printf("Found %d announcements:\n\n", len(announcements))
	
	for i, ann := range announcements {
		fmt.Printf("%d. Descriptor: %s\n", i+1, ann.Descriptor)
		
		// Show topic if known
		if topic, ok := topicMap[ann.TopicHash]; ok {
			fmt.Printf("   Topic: %s\n", topic)
		} else {
			fmt.Printf("   Topic hash: %s...\n", ann.TopicHash[:16])
		}
		
		fmt.Printf("   Category: %s, Size: %s\n", ann.Category, ann.SizeClass)
		
		// Show age
		age := time.Since(ann.ReceivedAt)
		fmt.Printf("   Received: %s ago", formatDuration(age))
		
		// Show source
		if ann.Source != "" {
			fmt.Printf(" (via %s)", ann.Source)
		}
		fmt.Println()
		
		// Check if expired
		if ann.IsExpired() {
			fmt.Println("   Status: Expired")
		} else {
			// Calculate time until expiration
			expiryTime := time.Unix(ann.Timestamp, 0).Add(time.Duration(ann.TTL) * time.Second)
			remaining := time.Until(expiryTime)
			fmt.Printf("   Expires in: %s\n", formatDuration(remaining))
		}
		
		fmt.Println()
	}
	
	// Show stats if not quiet
	if !quiet {
		total, byTopic, expired := annStore.GetStats()
		fmt.Printf("Store stats: %d total, %d expired, %d topics\n", total, expired, len(byTopic))
	}
	
	return nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}
}