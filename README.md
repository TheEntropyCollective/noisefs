# NoiseFS - Anonymous Distributed File Storage

NoiseFS is a privacy-focused distributed file system that implements the OFFSystem architecture on top of IPFS. It provides anonymous, efficient file storage with plausible deniability for all participants.

## Key Features

- **Anonymous Storage**: All blocks appear as random data through XOR anonymization
- **Efficient Block Reuse**: Smart caching minimizes storage overhead (~200% vs 900-2900% for traditional systems)
- **Plausible Deniability**: No original file content is ever stored
- **IPFS Integration**: Leverages IPFS's distributed network for resilience
- **Real-time Metrics**: Track efficiency and performance statistics

## Quick Start

### Prerequisites

1. Install [Go](https://golang.org/dl/) (1.19 or later)
2. Install [IPFS](https://docs.ipfs.tech/install/)
3. Start IPFS daemon:
   ```bash
   ipfs daemon
   ```

### Option 1: Web Interface (Recommended)

1. **Build and start the web UI:**
   ```bash
   go run ./cmd/webui
   ```

2. **Open your browser:**
   ```
   http://localhost:8080
   ```

3. **Upload a file:**
   - Select a file using the upload form
   - Choose block size (128KB default)
   - Click "Upload"
   - Save the descriptor CID for downloading

4. **Download a file:**
   - Enter the descriptor CID
   - Click "Download"
   - File will be saved to your Downloads folder

### Option 2: Command Line Interface

1. **Build the CLI:**
   ```bash
   go build ./cmd/noisefs
   ```

2. **Upload a file:**
   ```bash
   ./noisefs -upload myfile.txt
   ```

3. **Download a file:**
   ```bash
   ./noisefs -download <descriptor_cid> -output downloaded_file.txt
   ```

### Option 3: FUSE Filesystem (Transparent Integration)

1. **Mount NoiseFS as a filesystem:**
   ```bash
   go build ./cmd/noisefs-mount
   ./noisefs-mount -mount /mnt/noisefs
   ```

2. **Use standard file operations:**
   ```bash
   # Copy files (automatically uploaded and anonymized)
   cp document.pdf /mnt/noisefs/files/
   cp image.jpg /mnt/noisefs/files/photos/
   
   # List files
   ls /mnt/noisefs/files/
   
   # Read files (automatically downloaded and reconstructed)
   cat /mnt/noisefs/files/document.pdf > local_copy.pdf
   
   # View filesystem structure
   tree /mnt/noisefs/
   ```

3. **Unmount when done:**
   ```bash
   ./noisefs-mount -unmount -mount /mnt/noisefs
   ```

**Requirements for FUSE:**
- macFUSE (macOS) or FUSE (Linux) installed
- Build with FUSE support: `go build -tags fuse ./cmd/noisefs-mount`

## How It Works

NoiseFS implements the OFFSystem architecture:

1. **File Splitting**: Files are divided into 128KB blocks
2. **Anonymization**: Each block is XORed with a randomizer block
3. **Storage**: Only anonymized blocks are stored in IPFS
4. **Reconstruction**: Files are rebuilt by XORing stored blocks with randomizers
5. **Efficiency**: Popular randomizer blocks are reused across multiple files

## Performance Metrics

The system tracks several efficiency metrics:

- **Block Reuse Rate**: Percentage of randomizers reused from cache
- **Cache Hit Rate**: Efficiency of block caching
- **Storage Efficiency**: Overhead compared to original file size
- **Network Activity**: Upload/download operations

## Security Properties

- **Content Privacy**: Stored blocks appear as random data
- **Metadata Privacy**: No direct mapping between blocks and files
- **Plausible Deniability**: Hosts cannot prove what content they're storing
- **Distributed Risk**: No single point of failure or control

## Development

### Project Structure

```
cmd/
├── noisefs/    # CLI application
└── webui/      # Web interface

pkg/
├── blocks/     # File splitting and assembly
├── cache/      # Block caching system
├── descriptors/# File metadata management
├── ipfs/       # IPFS integration
├── noisefs/    # High-level client API
└── fuse/       # FUSE filesystem integration
```

### Running Tests

```bash
go test ./...
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

[Add your license here]

## Roadmap

- [x] Unit test coverage
- [x] FUSE filesystem integration
- [ ] Performance optimizations
- [ ] Privacy analysis and documentation
- [ ] Mobile applications
- [ ] Federation between IPFS networks

## Support

For questions, issues, or contributions, please [open an issue](https://github.com/your-repo/noisefs/issues) on GitHub.