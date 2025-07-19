# NoiseFS Documentation

Welcome to the NoiseFS documentation. NoiseFS is a distributed, privacy-preserving filesystem built on IPFS using the OFFSystem architecture.

## Getting Started

- **[Quick Start Guide](quickstart.md)** - Get up and running in 5 minutes
- **[Installation Guide](installation.md)** - Detailed installation instructions
- **[CLI Usage Guide](cli-usage.md)** - Command-line interface reference
- **[Configuration Guide](configuration.md)** - Configuration options and examples

## Architecture & Design

- **[Block Management](block-management.md)** - How NoiseFS splits and anonymizes files
- **[Storage Architecture](storage-architecture.md)** - Storage backend design and IPFS integration
- **[Cache System](cache-system.md)** - Caching strategy and implementation
- **[Altruistic Caching](altruistic-caching.md)** - Contribute spare capacity to network health
- **[Privacy Infrastructure](privacy-infrastructure.md)** - Privacy features and anonymity design
- **[FUSE Integration](fuse-integration.md)** - Mount NoiseFS as a regular filesystem

## Legal & Compliance

- **[Compliance Framework](compliance-framework.md)** - DMCA and legal compliance
- **[Legal Analysis](legal-analysis.md)** - Legal considerations and protections
- **[Privacy Analysis](privacy-analysis-comprehensive.md)** - Comprehensive privacy analysis

## Project Information

- **[Evolution Analysis](evolution-analysis.md)** - Development history and decisions
- **[Future Optimizations](future-optimizations.md)** - Planned features and enhancements
- **[TODO](todo.md)** - Current development tasks
- **[Worklog](worklog.md)** - Development progress log

## Quick Links

### For Users
1. [Install NoiseFS](installation.md)
2. [Upload your first file](quickstart.md#upload-your-first-file)
3. [Configure privacy settings](configuration.md#privacy-configuration)
4. [Mount as filesystem](quickstart.md#mount-as-filesystem-optional)

### For Developers
1. [Build from source](installation.md#installing-from-source)
2. [Architecture overview](block-management.md)
3. [Run benchmarks](evolution-analysis.md#performance-benchmarks)

### For System Administrators
1. [Production deployment](installation.md#post-installation-setup)
2. [Configuration reference](configuration.md)
3. [Performance tuning](configuration.md#performance-configuration)

## Key Features

- **Privacy by Design** - Files are split and XOR'd with randomizer blocks
- **Plausible Deniability** - No original content stored, only anonymized blocks
- **Distributed Storage** - Built on IPFS for decentralized storage
- **Altruistic Caching** - Automatically contribute spare capacity to improve network health
- **FUSE Mounting** - Use NoiseFS like a regular filesystem
- **Configurable Privacy** - Three privacy levels for different use cases
- **Legal Compliance** - Built-in DMCA compliance framework

## Getting Help

- **Command Help**: Run `noisefs --help` or `noisefs [command] --help`
- **GitHub Issues**: Report bugs or request features at the [GitHub repository](https://github.com/TheEntropyCollective/noisefs)
- **Documentation**: You're reading it! Navigate using the links above

## Documentation Structure

```
docs/
├── README.md                          # This file
├── quickstart.md                      # 5-minute quick start
├── installation.md                    # Installation guide
├── cli-usage.md                       # CLI reference
├── configuration.md                   # Configuration reference
├── block-management.md                # Core anonymization system
├── storage-architecture.md            # Storage layer design
├── cache-system.md                    # Caching implementation
├── privacy-infrastructure.md          # Privacy features
├── fuse-integration.md                # FUSE filesystem
├── compliance-framework.md            # Legal compliance
├── legal-analysis.md                  # Legal considerations
├── privacy-analysis-comprehensive.md  # Privacy analysis
├── evolution-analysis.md              # Development history
├── future-optimizations.md            # Planned features
├── todo.md                           # Development tasks
└── worklog.md                        # Progress log
```

## Contributing

NoiseFS is open source. To contribute:

1. Read the architecture documentation
2. Check the [TODO list](todo.md) for current tasks
3. Submit pull requests to the [GitHub repository](https://github.com/TheEntropyCollective/noisefs)

## License

NoiseFS is released under an open source license. See the repository for details.