# Nexorious CLI Client - Command Reference

Complete reference guide for the Nexorious command-line interface.

## Installation

```bash
# Install from PyPI (when available)
pip install nexorious-cli

# Install from source
cd cli/
pip install -e .

# Verify installation
nexcmd --version
```

**Note**: The package name is `nexorious-cli` but the command you use is `nexcmd`.

## Global Options

These options are available for all commands:

```bash
--server URL        # Server URL (env: NEXORIOUS_SERVER)
--config DIR        # Config directory path (env: NEXORIOUS_CONFIG_DIR)
--format FORMAT     # Output format: table, json, csv (env: NEXORIOUS_FORMAT)
--verbose, -v       # Verbose output (env: NEXORIOUS_VERBOSE)
--quiet, -q         # Quiet output (env: NEXORIOUS_QUIET)
--no-color          # Disable colored output
--help, -h          # Show help
--version           # Show version
```

## Environment Variables

```bash
# Server configuration
export NEXORIOUS_SERVER="https://api.example.com"

# Output preferences  
export NEXORIOUS_FORMAT="json"
export NEXORIOUS_VERBOSE="true"
export NEXORIOUS_COLOR="false"
export NEXORIOUS_PAGE_SIZE="50"

# Authentication
export NEXORIOUS_AUTO_REFRESH="true"

# Import settings
export NEXORIOUS_AUTO_DOWNLOAD_COVERS="false"
export NEXORIOUS_BATCH_SIZE="25"
```

## Commands

### Authentication Commands

#### `nexcmd auth login [USERNAME]`
Login to the server.

```bash
# Interactive login
nexcmd auth login

# Login with username
nexcmd auth login admin

```

#### `nexcmd auth logout`
Logout from the current server.

```bash
nexcmd auth logout
```

#### `nexcmd auth whoami`
Show current user information.

```bash
nexcmd auth whoami
```

#### `nexcmd auth setup`
Initial admin user setup (first run only).

```bash
nexcmd auth setup
```


#### Configuration Management

```bash
# Show current configuration
nexcmd auth config show

# Get specific config value
nexcmd auth config get server.url

# Set configuration value
nexcmd auth config set output.page_size 50
nexcmd auth config set server.timeout 60
```

### Games Commands - Global Game Database Operations

#### `nexcmd games search QUERY [OPTIONS]`
Search IGDB for games to discover new titles.

```bash
# Basic search
nexcmd games search "zelda breath of the wild"

# Limit results
nexcmd games search "final fantasy" --limit 5

# Different output format
nexcmd games search "mario" --format json
```

Options:
- `--limit N` - Maximum results (default: 10)

#### `nexcmd games list [OPTIONS]`
List games in the global database.

```bash
# List all games in database
nexcmd games list

# Search within database
nexcmd games list --search "zelda"

# Filter by genre
nexcmd games list --genre "RPG"

# With pagination
nexcmd games list --page 2 --per-page 50
```

Options:
- `--page N` - Page number (default: 1)
- `--per-page N` - Items per page (default: 20)
- `--search QUERY` - Search query
- `--genre GENRE` - Filter by genre
- `--developer DEVELOPER` - Filter by developer
- `--publisher PUBLISHER` - Filter by publisher

#### `nexcmd games show GAME_ID`
Show detailed information about a game in the global database.

```bash
nexcmd games show abc123-def456
nexcmd games show abc123-def456 --format json
```

#### `nexcmd games import IGDB_ID [OPTIONS]`
Import a game from IGDB to the global database.

```bash
# Basic import
nexcmd games import 1942

# Import with cover art download
nexcmd games import 1942 --download-cover-art
```

Options:
- `--download-cover-art` - Download cover art during import (default: true)
- `--custom-overrides KEY=VALUE` - Override specific fields

### User Games Commands - Personal Collection Management

#### `nexcmd usergames add GAME_ID [OPTIONS]`
Add a game from the global database to your personal collection.

```bash
# Add game to collection with basic status
nexcmd usergames add abc123-def456 --status owned

# Add with platform and storefront
nexcmd usergames add abc123-def456 --status owned --platform pc --storefront steam

# Add with detailed info
nexcmd usergames add abc123-def456 --status owned --platform pc --hours-played 25 --notes "Great game!"
```

Options:
- `--status STATUS` - Ownership status (owned, borrowed, rented, subscription)
- `--platform PLATFORM` - Platform name
- `--storefront STOREFRONT` - Storefront name
- `--hours-played N` - Hours played
- `--notes TEXT` - Personal notes

#### `nexcmd usergames list [OPTIONS]`
List your personal game collection.

```bash
# Basic collection list
nexcmd usergames list

# Filter by status
nexcmd usergames list --filter "status=owned"
nexcmd usergames list --filter "status=playing"

# Filter by platform
nexcmd usergames list --filter "platform=pc"

# Search within collection
nexcmd usergames list --search "zelda"

# With pagination and formatting
nexcmd usergames list --page 2 --format json
```

Options:
- `--page N` - Page number (default: 1)
- `--per-page N` - Items per page (default: 20)
- `--search QUERY` - Search query
- `--filter KEY=VALUE` - Filter results
- `--sort FIELD` - Sort by field

#### `nexcmd usergames show USER_GAME_ID`
Show detailed collection item information.

```bash
nexcmd usergames show user-game-123
nexcmd usergames show user-game-123 --format json
```

#### `nexcmd usergames update USER_GAME_ID [OPTIONS]`
Update collection item.

```bash
nexcmd usergames update user-game-123 --status completed --hours-played 50 --rating 9.5
```

Options:
- `--status STATUS` - Play status
- `--hours-played N` - Hours played
- `--rating N` - Personal rating (0-10)
- `--notes TEXT` - Personal notes

#### `nexcmd usergames progress USER_GAME_ID [OPTIONS]`
Update game progress.

```bash
# Update status and hours
nexcmd usergames progress user-game-123 --status playing --hours 25

# Just update status
nexcmd usergames progress user-game-123 --status completed
```

Options:
- `--status STATUS` - Play status
- `--hours N` - Hours played

#### Platform Management for Collection Items

```bash
# List platforms for a collection item
nexcmd usergames platforms list user-game-123

# Add platform to collection item
nexcmd usergames platforms add user-game-123 --platform pc --storefront steam

# Update platform association
nexcmd usergames platforms update user-game-123 platform-assoc-456 --storefront gog

# Remove platform from collection item
nexcmd usergames platforms remove user-game-123 platform-assoc-123
```

#### `nexcmd usergames remove USER_GAME_ID`
Remove game from your collection.

```bash
nexcmd usergames remove user-game-123
```

#### Bulk Collection Operations

```bash
# Bulk update status
nexcmd usergames bulk-update --games game1,game2,game3 --status completed

# Bulk add platforms
nexcmd usergames bulk-add-platforms --games game1,game2 --platform pc --storefront steam

# Bulk remove platforms
nexcmd usergames bulk-remove-platforms --games game1,game2 --platform-associations assoc1,assoc2

# Bulk delete
nexcmd usergames bulk-delete game1,game2,game3
```

#### `nexcmd usergames stats`
Show personal collection statistics.

```bash
nexcmd usergames stats
nexcmd usergames stats --format json
```

#### Game Aliases Management (Admin)

```bash
# List aliases for a game
nexcmd games aliases list game-123

# Create alias for a game  
nexcmd games aliases create game-123 "Alternative Title"

# Delete alias
nexcmd games aliases delete game-123 alias-456
```

#### Metadata Operations (Admin)

```bash
# Show metadata status
nexcmd games metadata status game-123

# Refresh game metadata
nexcmd games metadata refresh game-123 --force

# Populate missing metadata
nexcmd games metadata populate game-123

# Bulk metadata operations
nexcmd games metadata bulk --operation refresh --games game1,game2,game3
```

#### Cover Art Operations (Admin)

```bash
# Download cover art for a game
nexcmd games cover-art download game-123

# Bulk download cover art
nexcmd games cover-art bulk-download --games game1,game2 --skip-existing
```

### Tags Management Commands

#### `nexcmd tags list [OPTIONS]`
List your tags.

```bash
# List all tags
nexcmd tags list

# Include game counts
nexcmd tags list --include-game-count

# Pagination
nexcmd tags list --page 2 --per-page 25
```

#### `nexcmd tags create NAME [OPTIONS]`
Create a new tag.

```bash
# Basic tag
nexcmd tags create "RPG"

# Tag with color and description
nexcmd tags create "JRPG" \
  --color "#ff0000" \
  --description "Japanese Role-Playing Games"
```

#### `nexcmd tags update TAG_ID [OPTIONS]`
Update existing tag.

```bash
nexcmd tags update tag-123 --name "Action RPG" --color "#00ff00"
```

#### Tag Assignment

```bash
# Assign single tag
nexcmd tags assign user-game-123 tag-456

# Assign multiple tags
nexcmd tags assign user-game-123 tag-456 tag-789 tag-101

# Remove tags
nexcmd tags remove user-game-123 tag-456 tag-789
```

#### Bulk Tag Operations

```bash
# Bulk assign tags to multiple games
nexcmd tags bulk-assign \
  --games game1,game2,game3 \
  --tags tag1,tag2

# Bulk remove tags
nexcmd tags bulk-remove \
  --games game1,game2 \
  --tags tag1
```

#### `nexcmd tags stats`
Show tag usage statistics.

```bash
nexcmd tags stats
nexcmd tags stats --format json
```

### Wishlist Management Commands

#### `nexcmd wishlist list [OPTIONS]`
List wishlist items.

```bash
nexcmd wishlist list
nexcmd wishlist list --format table
nexcmd wishlist list --page 2
```

#### `nexcmd wishlist add GAME_ID`
Add game to wishlist.

```bash
nexcmd wishlist add abc123-def456
```

#### `nexcmd wishlist remove GAME_ID`
Remove game from wishlist.

```bash
nexcmd wishlist remove abc123-def456
```

#### `nexcmd wishlist clear`
Clear entire wishlist.

```bash
nexcmd wishlist clear
```

### Import Commands

#### Basic Import Operations

```bash
# Import from Steam
nexcmd import steam --steam-id 76561198000000000

# Import from CSV file
nexcmd import csv my-games.csv --platform pc

# Import from Darkadia
nexcmd import darkadia --api-key YOUR_API_KEY
```

#### Import Job Management

```bash
# List import jobs
nexcmd import jobs list

# Show job details
nexcmd import jobs show job-123

# Cancel running job
nexcmd import jobs cancel job-123

# Show import history
nexcmd import history
nexcmd import history --format json

# List available import sources
nexcmd import sources
```

#### Batch Import Operations (Darkadia)

```bash
# Start batch auto-match session
nexcmd import batch start auto-match --session-name "my-import"

# Check session status
nexcmd import batch status session-123

# Process next batch item
nexcmd import batch next session-123 --action accept
nexcmd import batch next session-123 --action reject
nexcmd import batch next session-123 --action manual --game-id abc123

# Cancel batch session
nexcmd import batch cancel session-123
```

#### Platform Resolution

```bash
# Get platform suggestions
nexcmd import resolve-suggestions \
  --platform "unknown-platform" \
  --storefront "steam"

# List pending resolutions
nexcmd import pending-resolutions
nexcmd import pending-resolutions --page 2

# Resolve single platform
nexcmd import resolve \
  --import-id import-123 \
  --platform-id platform-456 \
  --storefront-id storefront-789

# Bulk resolve platforms
nexcmd import bulk-resolve --file resolutions.json

# Check platform-storefront compatibility
nexcmd import compatibility-check platform-123 storefront-456
```

### Admin Commands

#### User Management

```bash
# List all users
nexcmd admin users list

# Create new user
nexcmd admin users create johndoe \
  --password secret123 \
  --admin

# Show user details
nexcmd admin users show user-123

# Update user
nexcmd admin users update user-123 \
  --username john.doe \
  --admin true

# Delete user (with impact preview)
nexcmd admin users deletion-impact user-123
nexcmd admin users delete user-123

# Reset password
nexcmd admin users reset-password user-123 \
  --password newpassword123
```

#### Platform Management

```bash
# List platforms
nexcmd admin platforms list
nexcmd admin platforms list --source official
nexcmd admin platforms list --active-only false

# Create platform
nexcmd admin platforms create \
  --name "Steam Deck" \
  --display-name "Steam Deck" \
  --icon-url "https://example.com/icon.png"

# Show platform details
nexcmd admin platforms show platform-123

# Update platform
nexcmd admin platforms update platform-123 \
  --display-name "Updated Name"

# Delete platform
nexcmd admin platforms delete platform-123

# Manage default storefront
nexcmd admin platforms default-storefront platform-123 storefront-456
```

#### Platform-Storefront Associations

```bash
# Associate platform with storefront
nexcmd admin platforms associate platform-123 storefront-456

# Remove association
nexcmd admin platforms dissociate platform-123 storefront-456
```

#### Logo Management

```bash
# Upload platform logo
nexcmd admin platforms upload-logo platform-123 \
  --theme light \
  --file logo.png

# Delete logo
nexcmd admin platforms delete-logo platform-123 --theme dark

# List logos
nexcmd admin platforms list-logos platform-123
```

#### Storefront Management

```bash
# List storefronts
nexcmd admin storefronts list

# Create storefront
nexcmd admin storefronts create \
  --name "new-store" \
  --display-name "New Store" \
  --base-url "https://store.example.com"

# Update storefront
nexcmd admin storefronts update storefront-123 \
  --display-name "Updated Store"

# Delete storefront
nexcmd admin storefronts delete storefront-123
```

#### System Operations

```bash
# Load seed data
nexcmd admin seed-data --version 2.0.0

# Run cleanup operations
nexcmd admin cleanup
```

#### Statistics

```bash
# Platform usage statistics
nexcmd admin stats platforms

# Storefront usage statistics
nexcmd admin stats storefronts

# Both with JSON output
nexcmd admin stats platforms --format json
```

## Configuration Examples

### Main Configuration

Edit `~/.config/nexorious/config.toml`:

```toml
[server]
url = "http://localhost:8000"
timeout = 30

[output]
default_format = "table"
page_size = 25
color = true
pager = "auto"

[import]
default_platform_resolution = "suggest"
batch_size = 50

[ui]
confirm_destructive = true
show_progress = true
```

## Common Workflows

### Initial Setup

```bash
# First time setup
nexcmd auth setup
nexcmd auth login admin

```

### Game Collection Management

```bash
# 1. Search IGDB for games
nexcmd games search "zelda breath"

# 2. Import game to global database (if not already exists)
nexcmd games import 1942

# 3. Add game to personal collection
nexcmd usergames add abc123-def456 --status owned --platform switch

# 4. Tag games in collection
nexcmd tags create "Open World" --color "#00ff00"
nexcmd tags assign user-game-123 tag-456

# 5. Track progress
nexcmd usergames progress user-game-123 --status playing --hours 10

# 6. View collection
nexcmd usergames list --filter "status=playing"
```

### Import Workflow

```bash
# Start Steam import
nexcmd import steam --steam-id 76561198000000000

# Monitor progress
nexcmd import status job-123

# Resolve any platform conflicts
nexcmd import pending-resolutions
nexcmd import resolve --import-id import-456 --platform-id platform-123
```

### Admin Tasks

```bash
# User management
nexcmd admin users create newuser --password temp123
nexcmd admin users list

# Platform management
nexcmd admin platforms list
nexcmd admin seed-data

# View statistics
nexcmd admin stats platforms
```

## Troubleshooting

### Authentication Issues

```bash
# Check authentication status
nexcmd auth whoami

# Clear and re-authenticate
nexcmd auth logout
nexcmd auth login

# Check token expiration
nexcmd auth config get session.tokens.token_expires_at
```

### Configuration Issues

```bash
# Show effective configuration
nexcmd auth config show

# Reset to defaults
rm -rf ~/.config/nexorious
rm -rf ~/.local/share/nexorious/auth

# Verify XDG directories
nexcmd --verbose auth config show
```

### Connection Issues

```bash
# Test connectivity
nexcmd --server http://localhost:8000 auth whoami

# Use different timeout
nexcmd auth config set server.timeout 60

# Check server URL
nexcmd auth config get server.url
```

For more help, run any command with `--help` or `-h`.