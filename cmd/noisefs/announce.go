package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/dht"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/pubsub"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
	shell "github.com/ipfs/go-ipfs-api"
)

// announceCommand handles the announce subcommand
func announceCommand(args []string, ipfsClient *ipfs.Client, shell *shell.Shell, quiet bool, jsonOutput bool) error {
	// Create flag set for announce command
	flagSet := flag.NewFlagSet("announce", flag.ExitOnError)
	
	var (
		topic     = flagSet.String("topic", "", "Topic for the announcement (required)")
		tags      = flagSet.String("tags", "", "Comma-separated tags for discovery")
		ttl       = flagSet.Duration("ttl", 24*time.Hour, "Time to live for announcement")
		autoTags  = flagSet.Bool("auto-tags", true, "Automatically extract tags from file")
		realtime  = flagSet.Bool("realtime", true, "Also publish to PubSub for real-time delivery")
		help      = flagSet.Bool("help", false, "Show help for announce command")
	)
	
	// Custom usage
	flagSet.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: noisefs announce <file> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Announce a file to a topic for discovery by others.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flagSet.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  noisefs announce myfile.pdf --topic \"documents/research\"\n")
		fmt.Fprintf(os.Stderr, "  noisefs announce video.mp4 --topic \"movies/scifi\" --tags \"4k,remastered\"\n")
	}
	
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	
	if *help || flagSet.NArg() == 0 {
		flagSet.Usage()
		return nil
	}
	
	// Get file path
	filePath := flagSet.Arg(0)
	
	// Validate inputs
	if *topic == "" {
		return fmt.Errorf("topic is required")
	}
	
	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to access file: %w", err)
	}
	
	logger := logging.GetGlobalLogger().WithComponent("announce")
	
	// First, upload the file to get descriptor
	if !quiet {
		fmt.Printf("Uploading %s to NoiseFS...\n", filePath)
	}
	
	// Create descriptor store
	descStore, err := descriptors.NewStore(ipfsClient)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	// Upload file (simplified - in real implementation would use full upload flow)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	// Store file and get CID (simplified)
	cid, err := ipfsClient.Add(file)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	
	// Create descriptor (simplified - would normally include proper block structure)
	descriptor := descriptors.NewDescriptor(fileInfo.Name(), fileInfo.Size(), 131072)
	descriptor.AddBlockPair(cid, cid) // Simplified for demo
	
	// Save descriptor
	descriptorCID, err := descStore.Save(descriptor)
	if err != nil {
		return fmt.Errorf("failed to save descriptor: %w", err)
	}
	
	if !quiet {
		fmt.Printf("Created descriptor: %s\n", descriptorCID)
	}
	
	// Create announcement
	creator := announce.NewCreator()
	
	// Parse tags
	var tagList []string
	if *tags != "" {
		tagList = strings.Split(*tags, ",")
		for i, tag := range tagList {
			tagList[i] = strings.TrimSpace(tag)
		}
	}
	
	// Create announcement options
	opts := announce.CreateOptions{
		Topic:    *topic,
		Tags:     tagList,
		TTL:      *ttl,
		AutoTags: *autoTags,
	}
	
	// Create announcement with file metadata
	announcement, err := creator.CreateFromFile(descriptorCID, filePath, opts)
	if err != nil {
		return fmt.Errorf("failed to create announcement: %w", err)
	}
	
	// Publish to DHT
	if !quiet {
		fmt.Printf("Publishing announcement to topic: %s\n", *topic)
		fmt.Printf("Topic hash: %s\n", announcement.TopicHash)
	}
	
	// Create DHT publisher
	pubConfig := dht.PublisherConfig{
		IPFSClient:  ipfsClient,
		IPFSShell:   shell,
		PublishRate: 1 * time.Minute,
	}
	
	publisher, err := dht.NewPublisher(pubConfig)
	if err != nil {
		return fmt.Errorf("failed to create publisher: %w", err)
	}
	
	ctx := context.Background()
	if err := publisher.Publish(ctx, announcement); err != nil {
		return fmt.Errorf("failed to publish announcement: %w", err)
	}
	
	// Also publish to PubSub if requested
	if *realtime {
		rtPublisher, err := pubsub.NewRealtimePublisher(shell)
		if err != nil {
			logger.Warn("Failed to create realtime publisher", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			if err := rtPublisher.Publish(ctx, announcement); err != nil {
				logger.Warn("Failed to publish to PubSub", map[string]interface{}{
					"error": err.Error(),
				})
			} else if !quiet {
				fmt.Println("Published to real-time PubSub channel")
			}
		}
	}
	
	// Output results
	if jsonOutput {
		result := map[string]interface{}{
			"success":       true,
			"descriptor":    descriptorCID,
			"topic":         *topic,
			"topic_hash":    announcement.TopicHash,
			"tags":          announcement.TagBloom != "",
			"ttl":           announcement.TTL,
			"realtime":      *realtime,
		}
		util.PrintJSON(result)
	} else if !quiet {
		fmt.Println("\n✓ Announcement published successfully!")
		fmt.Printf("Descriptor: %s\n", descriptorCID)
		fmt.Printf("Topic: %s (hash: %s...)\n", *topic, announcement.TopicHash[:16])
		if len(tagList) > 0 {
			fmt.Printf("Tags: %s\n", strings.Join(tagList, ", "))
		}
		fmt.Printf("Expires in: %v\n", *ttl)
	}
	
	return nil
}