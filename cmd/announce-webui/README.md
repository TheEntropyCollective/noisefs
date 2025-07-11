# NoiseFS Announcement Web UI

A web interface for browsing and searching NoiseFS announcements with real-time updates.

## Features

- **Real-time Updates**: WebSocket integration for live announcement streaming
- **Topic Browser**: Hierarchical topic navigation with subscription management
- **Advanced Search**: Filter by tags, categories, size classes, and time ranges
- **Responsive Design**: Mobile-friendly interface with dark mode support
- **Privacy-Preserving**: All operations maintain NoiseFS privacy guarantees

## Usage

### Starting the Web UI

```bash
# Default (port 8080)
announce-webui

# Custom port
announce-webui --port 9090

# Custom IPFS endpoint
announce-webui --ipfs http://127.0.0.1:5001

# Enable debug logging
announce-webui --debug
```

### Web Interface Pages

1. **Recent Announcements** (`/`)
   - Live feed of recent announcements
   - Filter by category and size class
   - Real-time WebSocket updates

2. **Topic Browser** (`/topics`)
   - Navigate topic hierarchy
   - Subscribe/unsubscribe from topics
   - View announcements per topic

3. **Search** (`/search`)
   - Advanced search with multiple criteria
   - Tag inclusion/exclusion
   - Time-based filtering
   - Relevance-based sorting

## API Endpoints

### REST API

- `GET /api/announcements` - List recent announcements
- `GET /api/topics` - Browse topic hierarchy
- `POST /api/topics/:path/subscribe` - Subscribe to topic
- `POST /api/topics/:path/unsubscribe` - Unsubscribe from topic
- `GET /api/subscriptions` - List active subscriptions
- `POST /api/search` - Search announcements
- `GET /api/stats` - Get system statistics

### WebSocket

- `ws://localhost:8080/api/ws` - Real-time announcement stream

## Configuration

The web UI respects NoiseFS configuration from:
- `~/.noisefs/config.yaml`
- Environment variables
- Command line flags

## Security Notes

- The web UI only exposes read operations
- No ability to create announcements via web
- Respects all NoiseFS privacy settings
- WebSocket connections are local-only by default

## Development

### Building

```bash
cd cmd/announce-webui
go build
```

### Frontend Assets

Static assets are located in:
- `templates/` - HTML templates
- `static/css/` - Stylesheets
- `static/js/` - JavaScript files

### Adding Features

1. Update backend handlers in `main.go`
2. Add HTML templates as needed
3. Update JavaScript for dynamic behavior
4. Maintain responsive design principles

## Troubleshooting

### Connection Issues

If the web UI cannot connect:
1. Verify IPFS is running: `ipfs daemon`
2. Check IPFS API port: default is 5001
3. Ensure NoiseFS daemon is running
4. Check firewall settings

### Missing Announcements

If no announcements appear:
1. Verify subscriptions are active
2. Check announcement sources are online
3. Review filter settings
4. Check browser console for errors

### Performance

For better performance:
1. Limit simultaneous subscriptions
2. Use search filters effectively
3. Consider running dedicated IPFS node
4. Monitor WebSocket connection status