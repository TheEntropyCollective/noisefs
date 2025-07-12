# NoiseFS Topic Configuration

## Overview

The topic hierarchy in NoiseFS is defined in `topics.json`. This file contains a comprehensive taxonomy for organizing content shared through the network.

## Topic Structure

Topics follow a hierarchical structure using forward slashes (/) as separators:
- `software` - Root category
- `software/opensource` - Subcategory
- `software/opensource/linux` - Deeper nesting

## Customizing Topics

### Adding New Topics

To add new topics, edit `topics.json` following this structure:

```json
{
  "topics": {
    "your-category": {
      "description": "Description of your category",
      "children": {
        "subcategory": {
          "description": "Subcategory description",
          "children": {
            "deeper-level": {
              "description": "Even deeper categorization"
            }
          }
        }
      }
    }
  }
}
```

### Best Practices

1. **Use descriptive names**: Choose clear, lowercase names with hyphens for spaces
2. **Add descriptions**: Help users understand what content belongs in each category
3. **Logical hierarchy**: Organize topics from general to specific
4. **Avoid over-nesting**: Try to limit depth to 3-4 levels for usability

## Default Categories

The default taxonomy includes:

- **software**: Open source projects, tools, libraries, containers
- **data**: Scientific datasets, ML models, geographic data
- **media**: Audio, video, images, 3D assets
- **documents**: Books, research papers, reference materials
- **education**: Courses, tutorials, lectures
- **archives**: Historical documents, web archives, cultural heritage

## Dynamic Topic Creation

Topics are also created automatically when users announce content with new topic paths. This allows the taxonomy to grow organically based on actual usage.

## Subscription Behavior

- Subscribing to a parent topic includes all child topics
- For example, subscribing to `software` includes all software-related announcements
- Users can subscribe to specific subtopics for more focused notifications

## Examples

Good topic paths:
- `software/opensource/linux/debian-based`
- `data/scientific/astronomy/exoplanets`
- `media/audio/podcasts/technology`
- `documents/books/public-domain/classics`

Avoid:
- Single-word topics without context
- Overly specific paths that won't be reused
- Mixing different categorization schemes