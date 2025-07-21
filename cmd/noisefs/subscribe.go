package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/config"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/dht"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/pubsub"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/security"
	"github.com/TheEntropyCollective/noisefs/pkg/announce/store"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
	shell "github.com/ipfs/go-ipfs-api"
)

// subscribeCommand handles the subscribe subcommand
func subscribeCommand(args []string, storageManager *storage.Manager, shell *shell.Shell, quiet bool, jsonOutput bool) error {
	flagSet := flag.NewFlagSet("subscribe", flag.ExitOnError)

	var (
		list    = flagSet.Bool("list", false, "List current subscriptions")
		remove  = flagSet.Bool("remove", false, "Remove subscription")
		monitor = flagSet.Bool("monitor", false, "Start monitoring subscriptions")
		help    = flagSet.Bool("help", false, "Show help for subscribe command")
	)

	flagSet.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: noisefs subscribe [topic-pattern] [options]\n\n")
		fmt.Fprintf(os.Stderr, "Subscribe to announcement topics for discovery.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flagSet.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  noisefs subscribe \"movies/scifi\"          # Subscribe to sci-fi movies\n")
		fmt.Fprintf(os.Stderr, "  noisefs subscribe \"documents/*\"           # Subscribe to all documents\n")
		fmt.Fprintf(os.Stderr, "  noisefs subscribe --list                  # List subscriptions\n")
		fmt.Fprintf(os.Stderr, "  noisefs subscribe --monitor               # Start monitoring\n")
	}

	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if *help {
		flagSet.Usage()
		return nil
	}

	// Load subscription config
	configPath := filepath.Join(config.GetConfigDir(), "subscriptions.json")
	subConfig, err := config.LoadSubscriptions(configPath)
	if err != nil {
		// Create new config if doesn't exist
		subConfig = config.NewSubscriptions()
	}

	// Handle list command
	if *list {
		return listSubscriptions(subConfig, quiet, jsonOutput)
	}

	// Handle monitor command
	if *monitor {
		return monitorSubscriptions(subConfig, storageManager, shell, quiet)
	}

	// Get topic pattern
	if flagSet.NArg() == 0 {
		flagSet.Usage()
		return fmt.Errorf("topic pattern required")
	}

	topic := flagSet.Arg(0)

	// Handle remove
	if *remove {
		return removeSubscription(subConfig, topic, configPath, quiet, jsonOutput)
	}

	// Add subscription
	return addSubscription(subConfig, topic, configPath, quiet, jsonOutput)
}

func listSubscriptions(subConfig *config.Subscriptions, _ bool, jsonOutput bool) error {
	subs := subConfig.GetAll()

	if jsonOutput {
		result := map[string]interface{}{
			"subscriptions": subs,
		}
		util.PrintJSON(result)
		return nil
	}

	if len(subs) == 0 {
		fmt.Println("No subscriptions")
		return nil
	}

	fmt.Println("Current subscriptions:")
	for _, sub := range subs {
		fmt.Printf("  %s", sub.Topic)
		if sub.TopicHash != "" {
			fmt.Printf(" (hash: %s...)", sub.TopicHash[:16])
		}
		fmt.Println()
	}

	return nil
}

func addSubscription(subConfig *config.Subscriptions, topic string, configPath string, quiet bool, jsonOutput bool) error {
	// Add subscription
	topicHash := announce.HashTopic(topic)
	sub := config.Subscription{
		Topic:     topic,
		TopicHash: topicHash,
		Active:    true,
	}

	if err := subConfig.Add(sub); err != nil {
		return fmt.Errorf("failed to add subscription: %w", err)
	}

	// Save config
	if err := config.SaveSubscriptions(configPath, subConfig); err != nil {
		return fmt.Errorf("failed to save subscriptions: %w", err)
	}

	if jsonOutput {
		result := map[string]interface{}{
			"success":    true,
			"topic":      topic,
			"topic_hash": topicHash,
		}
		util.PrintJSON(result)
	} else if !quiet {
		fmt.Printf("✓ Subscribed to: %s\n", topic)
		fmt.Printf("Topic hash: %s\n", topicHash)
	}

	return nil
}

func removeSubscription(subConfig *config.Subscriptions, topic string, configPath string, quiet bool, jsonOutput bool) error {
	if err := subConfig.Remove(topic); err != nil {
		return fmt.Errorf("failed to remove subscription: %w", err)
	}

	// Save config
	if err := config.SaveSubscriptions(configPath, subConfig); err != nil {
		return fmt.Errorf("failed to save subscriptions: %w", err)
	}

	if jsonOutput {
		result := map[string]interface{}{
			"success": true,
			"removed": topic,
		}
		util.PrintJSON(result)
	} else if !quiet {
		fmt.Printf("✓ Unsubscribed from: %s\n", topic)
	}

	return nil
}

func monitorSubscriptions(subConfig *config.Subscriptions, storageManager *storage.Manager, sh *shell.Shell, quiet bool) error {
	// Create announcement store
	storeConfig := store.DefaultStoreConfig(filepath.Join(config.GetConfigDir(), "announcements"))
	annStore, err := store.NewStore(storeConfig)
	if err != nil {
		return fmt.Errorf("failed to create announcement store: %w", err)
	}
	defer annStore.Close()

	// Create subscribers
	dhtConfig := dht.SubscriberConfig{
		StorageManager: storageManager,
		IPFSShell:      sh,
		PollInterval:   30 * time.Second,
	}

	dhtSubscriber, err := dht.NewSubscriber(dhtConfig)
	if err != nil {
		return fmt.Errorf("failed to create DHT subscriber: %w", err)
	}

	rtSubscriber, err := pubsub.NewRealtimeSubscriber(sh)
	if err != nil {
		return fmt.Errorf("failed to create realtime subscriber: %w", err)
	}

	// Create security manager
	securityManager := security.NewManager(nil)
	defer securityManager.Close()

	// Create handler with security checks
	handler := func(ann *announce.Announcement) error {
		// Perform security checks
		sourceID := ann.TopicHash + ":" + ann.Nonce // Use topic+nonce as source ID
		if err := securityManager.CheckAnnouncement(ann, sourceID); err != nil {
			if !quiet {
				fmt.Printf("\n[%s] Announcement rejected: %s\n", time.Now().Format("15:04:05"), err)
			}
			return nil // Don't propagate security errors
		}

		// Store announcement
		if err := annStore.Add(ann, "monitor"); err != nil {
			return err
		}

		if !quiet {
			fmt.Printf("\n[%s] New announcement:\n", time.Now().Format("15:04:05"))
			fmt.Printf("  Descriptor: %s\n", ann.Descriptor)
			fmt.Printf("  Topic hash: %s...\n", ann.TopicHash[:16])
			fmt.Printf("  Category: %s, Size: %s\n", ann.Category, ann.SizeClass)

			// Find matching subscription
			for _, sub := range subConfig.GetAll() {
				if sub.TopicHash == ann.TopicHash {
					fmt.Printf("  Matched topic: %s\n", sub.Topic)
					break
				}
			}
		}

		return nil
	}

	// Subscribe to all topics
	for _, sub := range subConfig.GetAll() {
		if !sub.Active {
			continue
		}

		// Subscribe to DHT
		if err := dhtSubscriber.SubscribeHash(sub.TopicHash, handler); err != nil {
			logging.GetGlobalLogger().Warn("Failed to subscribe to DHT", map[string]interface{}{
				"topic": sub.Topic,
				"error": err.Error(),
			})
		}

		// Subscribe to PubSub
		if err := rtSubscriber.SubscribeHash(sub.TopicHash, handler); err != nil {
			logging.GetGlobalLogger().Warn("Failed to subscribe to PubSub", map[string]interface{}{
				"topic": sub.Topic,
				"error": err.Error(),
			})
		}

		if !quiet {
			fmt.Printf("Monitoring: %s\n", sub.Topic)
		}
	}

	// Start monitoring
	if err := dhtSubscriber.Start(); err != nil {
		return fmt.Errorf("failed to start DHT subscriber: %w", err)
	}

	if !quiet {
		fmt.Println("\nMonitoring for announcements... (Press Ctrl+C to stop)")
	}

	// Wait for interrupt
	<-make(chan struct{})

	return nil
}
