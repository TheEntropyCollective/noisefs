#!/bin/bash

# Create a test directory structure
mkdir -p test_data/documents
mkdir -p test_data/images

# Create some test files
echo "This is an important document about NoiseFS" > test_data/documents/important.txt
echo "NoiseFS is a privacy-preserving file system" > test_data/documents/readme.txt
echo "Technical documentation for the search feature" > test_data/documents/search_docs.txt
echo "Random content here" > test_data/images/photo.txt

# Create a test index
cat > test_index.json << EOF
{
  "documents/important.txt": {
    "DescriptorCID": "QmTest1",
    "FileSize": 45,
    "ModifiedAt": "2024-01-01T12:00:00Z"
  },
  "documents/readme.txt": {
    "DescriptorCID": "QmTest2",
    "FileSize": 44,
    "ModifiedAt": "2024-01-02T12:00:00Z"
  },
  "documents/search_docs.txt": {
    "DescriptorCID": "QmTest3",
    "FileSize": 48,
    "ModifiedAt": "2024-01-03T12:00:00Z"
  },
  "images/photo.txt": {
    "DescriptorCID": "QmTest4",
    "FileSize": 20,
    "ModifiedAt": "2024-01-04T12:00:00Z"
  }
}
EOF

echo "Test data created successfully!"
echo "You can now test search with:"
echo "  NOISEFS_SKIP_LEGAL_NOTICE=1 ./noisefs search -index test_index.json \"NoiseFS\""
echo "  NOISEFS_SKIP_LEGAL_NOTICE=1 ./noisefs search -index test_index.json --name \"*.txt\""
echo "  NOISEFS_SKIP_LEGAL_NOTICE=1 ./noisefs search -index test_index.json --stats"