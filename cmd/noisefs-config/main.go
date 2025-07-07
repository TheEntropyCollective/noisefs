package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/TheEntropyCollective/noisefs/pkg/config"
)

func main() {
	var (
		init_    = flag.Bool("init", false, "Initialize default configuration file")
		show     = flag.Bool("show", false, "Show current configuration")
		validate = flag.Bool("validate", false, "Validate configuration file")
		path     = flag.String("config", "", "Configuration file path (default: ~/.noisefs/config.json)")
	)

	flag.Parse()

	configPath := *path
	if configPath == "" {
		defaultPath, err := config.GetDefaultConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get default config path: %v\n", err)
			os.Exit(1)
		}
		configPath = defaultPath
	}

	if *init_ {
		initConfig(configPath)
	} else if *show {
		showConfig(configPath)
	} else if *validate {
		validateConfig(configPath)
	} else {
		flag.Usage()
	}
}

func initConfig(path string) {
	cfg := config.DefaultConfig()
	
	if err := cfg.SaveToFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save config: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Default configuration saved to: %s\n", path)
}

func showConfig(path string) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal config: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Configuration from %s:\n", path)
	fmt.Println(string(data))
}

func validateConfig(path string) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("Configuration at %s is valid\n", path)
}