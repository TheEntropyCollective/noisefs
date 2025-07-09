package bootstrap

import (
	"fmt"
)

// getBuiltinDatasets returns a map of predefined public domain datasets
func getBuiltinDatasets() map[string]*Dataset {
	datasets := map[string]*Dataset{
		"books": {
			Name:        "Classic Literature",
			Description: "Public domain books from Project Gutenberg",
			Directory:   "books",
			MaxFiles:    50,
			Sources: []DataSource{
				{
					URL:      "https://www.gutenberg.org/files/1342/1342-0.txt",
					Type:     "literature",
					Filename: "pride_and_prejudice.txt",
					Size:     735000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Jane Austen",
						"title":  "Pride and Prejudice",
						"year":   "1813",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/11/11-0.txt",
					Type:     "literature",
					Filename: "alice_in_wonderland.txt",
					Size:     164000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Lewis Carroll",
						"title":  "Alice's Adventures in Wonderland",
						"year":   "1865",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/1661/1661-0.txt",
					Type:     "literature",
					Filename: "sherlock_holmes_adventures.txt",
					Size:     590000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Arthur Conan Doyle",
						"title":  "The Adventures of Sherlock Holmes",
						"year":   "1892",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/74/74-0.txt",
					Type:     "literature",
					Filename: "tom_sawyer.txt",
					Size:     420000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Mark Twain",
						"title":  "The Adventures of Tom Sawyer",
						"year":   "1876",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/1080/1080-0.txt",
					Type:     "literature",
					Filename: "modest_proposal.txt",
					Size:     26000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Jonathan Swift",
						"title":  "A Modest Proposal",
						"year":   "1729",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/2701/2701-0.txt",
					Type:     "literature",
					Filename: "moby_dick.txt",
					Size:     1257000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Herman Melville",
						"title":  "Moby Dick",
						"year":   "1851",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/1232/1232-0.txt",
					Type:     "literature",
					Filename: "prince.txt",
					Size:     310000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Niccolò Machiavelli",
						"title":  "The Prince",
						"year":   "1532",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/345/345-0.txt",
					Type:     "literature",
					Filename: "dracula.txt",
					Size:     880000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"author": "Bram Stoker",
						"title":  "Dracula",
						"year":   "1897",
					},
				},
			},
		},
		"images": {
			Name:        "Public Domain Images",
			Description: "Historical images and artwork from Wikimedia Commons",
			Directory:   "images",
			MaxFiles:    30,
			Sources: []DataSource{
				{
					URL:      "https://upload.wikimedia.org/wikipedia/commons/thumb/e/ea/Van_Gogh_-_Starry_Night_-_Google_Art_Project.jpg/1280px-Van_Gogh_-_Starry_Night_-_Google_Art_Project.jpg",
					Type:     "artwork",
					Filename: "starry_night.jpg",
					Size:     245000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"artist": "Vincent van Gogh",
						"title":  "The Starry Night",
						"year":   "1889",
					},
				},
				{
					URL:      "https://upload.wikimedia.org/wikipedia/commons/thumb/0/0a/The_Great_Wave_off_Kanagawa.jpg/1280px-The_Great_Wave_off_Kanagawa.jpg",
					Type:     "artwork",
					Filename: "great_wave.jpg",
					Size:     180000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"artist": "Katsushika Hokusai",
						"title":  "The Great Wave off Kanagawa",
						"year":   "1831",
					},
				},
				{
					URL:      "https://upload.wikimedia.org/wikipedia/commons/thumb/e/ec/Mona_Lisa%2C_by_Leonardo_da_Vinci%2C_from_C2RMF_retouched.jpg/687px-Mona_Lisa%2C_by_Leonardo_da_Vinci%2C_from_C2RMF_retouched.jpg",
					Type:     "artwork",
					Filename: "mona_lisa.jpg",
					Size:     120000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"artist": "Leonardo da Vinci",
						"title":  "Mona Lisa",
						"year":   "1503",
					},
				},
				{
					URL:      "https://upload.wikimedia.org/wikipedia/commons/thumb/4/4d/The_School_of_Athens.jpg/1280px-The_School_of_Athens.jpg",
					Type:     "artwork",
					Filename: "school_of_athens.jpg",
					Size:     320000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"artist": "Raphael",
						"title":  "The School of Athens",
						"year":   "1511",
					},
				},
				{
					URL:      "https://upload.wikimedia.org/wikipedia/commons/thumb/5/5b/Michelangelo_-_Creation_of_Adam_%28cropped%29.jpg/1280px-Michelangelo_-_Creation_of_Adam_%28cropped%29.jpg",
					Type:     "artwork",
					Filename: "creation_of_adam.jpg",
					Size:     280000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"artist": "Michelangelo",
						"title":  "The Creation of Adam",
						"year":   "1512",
					},
				},
			},
		},
		"documents": {
			Name:        "Historical Documents",
			Description: "Important historical documents and speeches",
			Directory:   "documents",
			MaxFiles:    25,
			Sources: []DataSource{
				{
					URL:      "https://www.gutenberg.org/files/1/1-0.txt",
					Type:     "historical",
					Filename: "gettysburg_address.txt",
					Size:     2000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"title": "Gettysburg Address",
						"date":  "1863-11-19",
						"type":  "speech",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/147/147-0.txt",
					Type:     "historical",
					Filename: "aesop_fables.txt",
					Size:     350000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"title": "Aesop's Fables",
						"author": "Aesop",
						"type":  "fables",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/76/76-0.txt",
					Type:     "historical",
					Filename: "huck_finn.txt",
					Size:     600000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"title": "Adventures of Huckleberry Finn",
						"author": "Mark Twain",
						"year":   "1884",
					},
				},
				{
					URL:      "https://www.gutenberg.org/files/98/98-0.txt",
					Type:     "historical",
					Filename: "tale_of_two_cities.txt",
					Size:     800000,
					License:  "Public Domain",
					Metadata: map[string]string{
						"title": "A Tale of Two Cities",
						"author": "Charles Dickens",
						"year":   "1859",
					},
				},
			},
		},
		"code": {
			Name:        "Public Domain Code",
			Description: "Historical and reference code implementations",
			Directory:   "code",
			MaxFiles:    15,
			Sources: []DataSource{
				{
					URL:      "https://raw.githubusercontent.com/torvalds/linux/master/kernel/fork.c",
					Type:     "source",
					Filename: "linux_fork.c",
					Size:     75000,
					License:  "GPL-2.0",
					Metadata: map[string]string{
						"language": "C",
						"project":  "Linux Kernel",
						"module":   "Process Management",
					},
				},
				{
					URL:      "https://raw.githubusercontent.com/python/cpython/main/Python/bltinmodule.c",
					Type:     "source",
					Filename: "python_builtins.c",
					Size:     95000,
					License:  "Python Software Foundation License",
					Metadata: map[string]string{
						"language": "C",
						"project":  "CPython",
						"module":   "Built-in Functions",
					},
				},
			},
		},
		"mixed": {
			Name:        "Mixed Content",
			Description: "A balanced mix of all content types",
			Directory:   "mixed",
			MaxFiles:    100,
			Sources:     []DataSource{},
		},
	}
	
	// Populate the mixed dataset
	expandMixedDataset(datasets)
	
	return datasets
}

// expandMixedDataset populates the mixed dataset with selections from other datasets
func expandMixedDataset(datasets map[string]*Dataset) {
	mixed := datasets["mixed"]
	
	// Add selections from each dataset
	for name, dataset := range datasets {
		if name == "mixed" {
			continue
		}
		
		// Add first few items from each dataset
		limit := 3
		if name == "books" {
			limit = 2 // Books are larger
		}
		
		for i, source := range dataset.Sources {
			if i >= limit {
				break
			}
			
			// Create a copy with updated directory path
			mixedSource := source
			mixedSource.Type = fmt.Sprintf("%s_%s", name, source.Type)
			mixed.Sources = append(mixed.Sources, mixedSource)
		}
	}
}

