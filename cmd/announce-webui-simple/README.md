# NoiseFS Announcement Web UI (Simple Version)

This is a simplified, standalone version of the NoiseFS announcement web UI that works with mock data for testing and demonstration purposes.

## Features

- Mock announcement data generation
- Working WebSocket connections for real-time updates
- Topic hierarchy browsing
- Search interface
- Subscription management (mock)

## Running the Web UI

```bash
cd cmd/announce-webui-simple
go build
./announce-webui-simple
```

The web UI will start on port 8080 by default.

## Accessing the Interface

- http://localhost:8080 - Recent announcements with real-time updates
- http://localhost:8080/topics - Browse topic hierarchy
- http://localhost:8080/search - Search interface

## Command Line Options

```bash
./announce-webui-simple -help
  -data string
        Data directory (default "./announce-data")
  -debug
        Enable debug logging
  -ipfs string
        IPFS API endpoint (default "http://127.0.0.1:5001")
  -port int
        Port to listen on (default 8080)
```

## Mock Data

This version generates mock announcements every 30 seconds to demonstrate the real-time WebSocket functionality. The mock data includes:

- Sample announcements for books and software
- Topic hierarchy (content/books, software/tools, etc.)
- Simulated subscriptions

## Development

This simple version is useful for:
- Testing the web UI without a full NoiseFS setup
- Frontend development and styling
- Demonstrating the interface to users
- Debugging WebSocket connections

To integrate with the real NoiseFS announcement system, use the full version in `cmd/announce-webui/` once the type compatibility issues are resolved.