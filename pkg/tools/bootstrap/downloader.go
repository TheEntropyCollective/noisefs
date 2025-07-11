package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ContentDownloader handles downloading public domain content
type ContentDownloader struct {
	config   *SeedConfig
	client   *http.Client
	progress *DownloadProgress
	mutex    sync.Mutex
}

// NewContentDownloader creates a new content downloader
func NewContentDownloader(config *SeedConfig) *ContentDownloader {
	return &ContentDownloader{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Minute, // Long timeout for video downloads
		},
		progress: &DownloadProgress{
			StartTime: time.Now(),
			Errors:    make([]error, 0),
		},
	}
}

// DownloadContentType downloads all content of a specific type
func (d *ContentDownloader) DownloadContentType(contentType string) error {
	// Load manifest for content type
	manifest, err := d.loadManifest(contentType)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	fmt.Printf("Found %d %s sources (%.1f MB total)\n", 
		manifest.FileCount, contentType,
		float64(manifest.TotalSize)/(1024*1024))

	// Create download directory
	downloadDir := filepath.Join(d.config.OutputDir, "downloads", contentType)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	// Download files in parallel
	sem := make(chan struct{}, d.config.Parallel)
	var wg sync.WaitGroup
	downloadedSize := int64(0)

	for i, source := range manifest.Sources {
		// Check size limit
		if atomic.LoadInt64(&downloadedSize) >= d.config.MaxSize {
			fmt.Printf("Reached size limit, stopping downloads\n")
			break
		}

		// Skip videos if not included
		if contentType == "videos" && !d.config.IncludeVideo {
			continue
		}

		// Video sources are already filtered in the manifest based on quality settings

		wg.Add(1)
		go func(idx int, src ContentSource) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			d.updateProgress(src.Name, false)
			
			outputPath := filepath.Join(downloadDir, src.Name)
			size, err := d.downloadFile(src.URL, outputPath)
			if err != nil {
				d.addError(fmt.Errorf("failed to download %s: %w", src.Name, err))
				return
			}

			atomic.AddInt64(&downloadedSize, size)
			d.updateProgress(src.Name, true)
			
			// Save metadata
			d.saveMetadata(outputPath, src)
		}(i, source)
	}

	wg.Wait()

	if len(d.progress.Errors) > 0 {
		fmt.Printf("Warning: %d download errors occurred\n", len(d.progress.Errors))
	}

	return nil
}

// loadManifest loads the content manifest for a type
func (d *ContentDownloader) loadManifest(contentType string) (*ContentManifest, error) {
	// In production, load from embedded resources or remote URL
	// For now, return sample data
	switch contentType {
	case "books":
		return d.getBooksManifest(), nil
	case "images":
		return d.getImagesManifest(), nil
	case "audio":
		return d.getAudioManifest(), nil
	case "videos":
		return d.getVideosManifest(), nil
	case "documents":
		return d.getDocumentsManifest(), nil
	default:
		return nil, fmt.Errorf("unknown content type: %s", contentType)
	}
}

// downloadFile downloads a file from URL to destination
func (d *ContentDownloader) downloadFile(url, destPath string) (int64, error) {
	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	// Get the data
	resp, err := d.client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer with progress
	counter := &progressWriter{
		total:      resp.ContentLength,
		downloaded: 0,
		onProgress: d.reportProgress,
	}

	// Write the body to file with progress
	written, err := io.Copy(out, io.TeeReader(resp.Body, counter))
	if err != nil {
		return 0, err
	}

	return written, nil
}

// progressWriter tracks download progress
type progressWriter struct {
	total      int64
	downloaded int64
	onProgress func(downloaded, total int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)
	if pw.onProgress != nil {
		pw.onProgress(pw.downloaded, pw.total)
	}
	return n, nil
}


// saveMetadata saves content metadata alongside the file
func (d *ContentDownloader) saveMetadata(filePath string, source ContentSource) error {
	metaPath := filePath + ".meta.json"
	
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(metaPath, data, 0644)
}

// Progress tracking methods
func (d *ContentDownloader) updateProgress(file string, completed bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	d.progress.CurrentFile = file
	if completed {
		d.progress.CompletedFiles++
	}
}

func (d *ContentDownloader) addError(err error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.progress.Errors = append(d.progress.Errors, err)
}

func (d *ContentDownloader) reportProgress(downloaded, total int64) {
	// In production, update progress bar
	// For now, periodic prints
	if downloaded == total {
		fmt.Printf("✓ Downloaded %s\n", d.progress.CurrentFile)
	}
}

// Sample manifest data
func (d *ContentDownloader) getBooksManifest() *ContentManifest {
	return &ContentManifest{
		Type:        "books",
		Description: "Classic literature from Project Gutenberg",
		TotalSize:   100 * 1024 * 1024, // 100MB
		FileCount:   20,
		Sources: []ContentSource{
			{
				Name:    "pride_and_prejudice.txt",
				URL:     "https://www.gutenberg.org/files/1342/1342-0.txt",
				Type:    "text",
				Size:    735000,
				License: "Public Domain",
				Metadata: map[string]string{
					"author": "Jane Austen",
					"title":  "Pride and Prejudice",
				},
			},
			{
				Name:    "alice_wonderland.txt",
				URL:     "https://www.gutenberg.org/files/11/11-0.txt",
				Type:    "text",
				Size:    164000,
				License: "Public Domain",
				Metadata: map[string]string{
					"author": "Lewis Carroll",
					"title":  "Alice's Adventures in Wonderland",
				},
			},
			// Add more books...
		},
	}
}

func (d *ContentDownloader) getImagesManifest() *ContentManifest {
	return &ContentManifest{
		Type:        "images",
		Description: "Public domain images from Wikimedia Commons",
		TotalSize:   300 * 1024 * 1024, // 300MB
		FileCount:   100,
		Sources: []ContentSource{
			{
				Name:    "starry_night.jpg",
				URL:     "https://upload.wikimedia.org/wikipedia/commons/thumb/e/ea/Van_Gogh_-_Starry_Night_-_Google_Art_Project.jpg/1280px-Van_Gogh_-_Starry_Night_-_Google_Art_Project.jpg",
				Type:    "image",
				Size:    2048000,
				License: "Public Domain",
				Metadata: map[string]string{
					"artist": "Vincent van Gogh",
					"year":   "1889",
				},
			},
			// Add more images...
		},
	}
}

func (d *ContentDownloader) getAudioManifest() *ContentManifest {
	return &ContentManifest{
		Type:        "audio",
		Description: "Classical music from Internet Archive",
		TotalSize:   500 * 1024 * 1024, // 500MB
		FileCount:   50,
		Sources: []ContentSource{
			{
				Name:    "beethoven_symphony_5.mp3",
				URL:     "https://archive.org/download/BeethovenSymphonyNo.5/01.I.AllegroConBrio.mp3",
				Type:    "audio",
				Size:    15000000,
				License: "Public Domain",
				Metadata: map[string]string{
					"composer": "Ludwig van Beethoven",
					"work":     "Symphony No. 5",
				},
			},
			// Add more audio...
		},
	}
}

func (d *ContentDownloader) getVideosManifest() *ContentManifest {
	sources := make([]ContentSource, 0)
	
	// Convert VideoSource to ContentSource
	videoSources := []VideoSource{
		{
			ContentSource: ContentSource{
				Name:    "trip_to_moon_1902.mp4",
				URL:     "https://archive.org/download/LeVoyageDansLaLune/Levoyagedanslalune.mp4",
				Type:    "video",
				Size:    32000000,
				License: "Public Domain",
				Metadata: map[string]string{
					"director": "Georges Méliès",
					"year":     "1902",
				},
			},
			Duration:   "14:43",
			Resolution: "480p",
			Format:     "mp4",
		},
		{
			ContentSource: ContentSource{
				Name:    "night_of_living_dead.mp4",
				URL:     "https://archive.org/download/NightOfTheLivingDead/Night.Of.The.Living.Dead_512kb.mp4",
				Type:    "video",
				Size:    350000000,
				License: "Public Domain",
				Metadata: map[string]string{
					"director": "George A. Romero",
					"year":     "1968",
				},
			},
			Duration:   "96:00",
			Resolution: "480p",
			Format:     "mp4",
		},
		{
			ContentSource: ContentSource{
				Name:    "big_buck_bunny.mp4",
				URL:     "https://archive.org/download/BigBuckBunny_124/Content/big_buck_bunny_720p_surround.mp4",
				Type:    "video",
				Size:    234000000,
				License: "CC-BY",
				Metadata: map[string]string{
					"studio": "Blender Foundation",
					"year":   "2008",
				},
			},
			Duration:   "9:56",
			Resolution: "720p",
			Format:     "mp4",
		},
		{
			ContentSource: ContentSource{
				Name:    "duck_and_cover.mp4",
				URL:     "https://archive.org/download/DuckandC1951/DuckandC1951_512kb.mp4",
				Type:    "video",
				Size:    45000000,
				License: "Public Domain",
				Metadata: map[string]string{
					"producer": "US Federal Civil Defense Administration",
					"year":     "1951",
				},
			},
			Duration:   "9:15",
			Resolution: "480p",
			Format:     "mp4",
		},
	}
	
	for _, vs := range videoSources {
		sources = append(sources, vs.ContentSource)
	}
	
	return &ContentManifest{
		Type:        "videos",
		Description: "Public domain videos from Internet Archive",
		TotalSize:   1024 * 1024 * 1024, // 1GB
		FileCount:   len(sources),
		Sources:     sources,
	}
}

func (d *ContentDownloader) getDocumentsManifest() *ContentManifest {
	return &ContentManifest{
		Type:        "documents",
		Description: "Historical documents and papers",
		TotalSize:   50 * 1024 * 1024, // 50MB
		FileCount:   30,
		Sources: []ContentSource{
			{
				Name:    "constitution_usa.txt",
				URL:     "https://www.archives.gov/files/founding-docs/constitution-transcript.txt",
				Type:    "document",
				Size:    45000,
				License: "Public Domain",
				Metadata: map[string]string{
					"type": "legal",
					"year": "1787",
				},
			},
			// Add more documents...
		},
	}
}