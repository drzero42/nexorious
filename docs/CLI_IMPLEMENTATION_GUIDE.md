# Nexorious CLI Client - Implementation Guide

This document provides implementation guidance, code patterns, and technical details for building the Nexorious CLI client according to the [CLI Specification](./CLI_SPECIFICATION.md).

## Project Structure

The CLI is a separate standalone project, not part of the backend codebase:

```
nexorious/                      # Project root
├── backend/                    # FastAPI backend
│   ├── app/
│   └── pyproject.toml
├── frontend/                   # SvelteKit frontend
│   ├── src/
│   └── package.json
├── cli/                        # ✅ Separate CLI project
│   ├── nexorious_cli/          # Python package (internal name)
│   │   ├── __init__.py
│   │   ├── main.py             # CLI entry point
│   │   ├── auth.py             # Authentication manager
│   │   ├── client.py           # HTTP client
│   │   ├── config.py           # Configuration manager
│   │   ├── formatter.py        # Output formatting
│   │   ├── exceptions.py       # Custom exceptions
│   │   └── commands/
│   │       ├── __init__.py
│   │       ├── auth.py         # Auth commands
│   │       ├── games.py        # Game commands
│   │       ├── tags.py         # Tags management commands
│   │       ├── wishlist.py     # Wishlist management commands
│   │       ├── import_cmd.py   # Import commands
│   │       └── admin.py        # Admin commands
│   ├── tests/                  # CLI-specific tests
│   ├── pyproject.toml          # CLI dependencies (separate from backend)
│   └── README.md               # CLI-specific documentation
└── docs/                       # Shared project documentation
    ├── CLI_SPECIFICATION.md
    ├── CLI_IMPLEMENTATION_GUIDE.md
    └── CLI_REFERENCE.md
```

**Important naming distinction:**
- **Package name**: `nexorious_cli` (internal Python package name)
- **Command name**: `nexcmd` (what users actually type in terminal)

The CLI communicates with the backend via HTTP API calls only - there are no direct imports between the CLI and backend codebases.

## Core Implementation Patterns

### Configuration Manager Pattern

The ConfigManager should implement XDG compliance:

```python
class ConfigManager:
    """XDG-compliant configuration manager."""
    
    def __init__(self, config_dir: Optional[str] = None):
        # Initialize XDG directories
        # Set up directory structure
        # Load configuration files
    
    def get(self, key: str, default=None, source: str = "merged"):
        """Get configuration value with precedence handling."""
    
    def save_auth_session(self, tokens: Dict, user_info: Dict):
        """Save authentication session with secure permissions."""
    
    def is_authenticated(self) -> bool:
        """Check authentication status with token validation."""
```

Key implementation requirements:
- XDG directory detection with fallbacks
- Secure file permissions (600 for tokens, 700 for auth dirs)
- Configuration precedence handling
- Token expiration validation

### HTTP Client Pattern

The HTTP client should handle authentication automatically:

```python
class NexoriousClient:
    """HTTP client with automatic authentication."""
    
    async def request(self, method: str, endpoint: str, **kwargs) -> httpx.Response:
        """Make authenticated request with auto-refresh."""
        # Add authentication headers
        # Make request
        # Handle 401 with token refresh
        # Retry once after refresh
```

Key implementation requirements:
- Async operations using httpx
- Automatic token refresh on 401
- Configurable timeouts
- File upload support
- Error handling and retries

### Command Base Pattern

All commands should inherit from a base command class:

```python
class BaseCommand(ABC):
    def __init__(self, client: NexoriousClient, config: ConfigManager):
        self.client = client
        self.config = config
    
    @abstractmethod
    def add_parser(self, subparsers) -> argparse.ArgumentParser:
        """Add command parser to subparsers"""
        
    @abstractmethod
    async def execute(self, args: argparse.Namespace) -> int:
        """Execute the command, return exit code"""
```

### Output Formatter Pattern

Support multiple output formats with color handling:

```python
class OutputFormatter:
    def __init__(self, format_type: str = 'table', use_color: bool = True):
        self.format_type = format_type
        self.use_color = use_color
    
    def format_table(self, data: List[Dict], headers: List[str] = None) -> str:
        """Format data as table"""
    
    def format_json(self, data: Any) -> str:
        """Format data as JSON"""
    
    def format_csv(self, data: List[Dict]) -> str:
        """Format data as CSV"""
```

## Authentication Implementation

### JWT Token Management

```python
class AuthManager:
    def __init__(self, config: ConfigManager):
        self.config = config
    
    async def login(self, username: str, password: str) -> bool:
        """Login and store tokens"""
        response = await self.client.request(
            "POST", "/auth/login",
            json={"username": username, "password": password},
            require_auth=False
        )
        
        if response.status_code == 200:
            data = response.json()
            self.config.save_auth_session(data, {"username": username})
            return True
        return False
    
    async def refresh_token(self) -> bool:
        """Refresh access token"""
        tokens = self.config.get_auth_tokens()
        if not tokens or not tokens.get("refresh_token"):
            return False
        
        response = await self.client.request(
            "POST", "/auth/refresh",
            json={"refresh_token": tokens["refresh_token"]},
            require_auth=False
        )
        
        if response.status_code == 200:
            # Update stored tokens
            return True
        return False
```

### First-Run Setup

```python
async def setup_admin(self):
    """Handle first-run admin setup"""
    # Check if setup needed
    status_response = await self.client.request(
        "GET", "/auth/setup/status", 
        require_auth=False
    )
    
    if status_response.json().get("needs_setup"):
        username = input("Enter admin username: ")
        password = getpass.getpass("Enter admin password: ")
        
        setup_response = await self.client.request(
            "POST", "/auth/setup/admin",
            json={"username": username, "password": password},
            require_auth=False
        )
        
        if setup_response.status_code == 201:
            print("Admin user created successfully")
            return True
    
    return False
```

## Configuration Implementation

### Environment Variable Handling

```python
def get_env_config() -> Dict[str, Any]:
    """Parse environment variables into config structure."""
    env_mappings = {
        'NEXORIOUS_SERVER': 'server.url',
        'NEXORIOUS_FORMAT': 'output.default_format',
        # ... more mappings
    }
    
    env_config = {}
    for env_var, config_key in env_mappings.items():
        value = os.environ.get(env_var)
        if value is not None:
            # Type conversion logic
            # Nested key assignment
```

### Configuration File Examples

#### Main Config (config.toml)
```toml
[server]
url = "http://localhost:8000"
timeout = 30

[output]
default_format = "table"
page_size = 20
color = true

[import]
batch_size = 50

[ui]
confirm_destructive = true
```


## API Service Implementation

### Service Layer Pattern

Create service classes for each API domain to match CLI command structure:

```python
class GamesService:
    """Service for global game database operations (/api/games/*)"""
    def __init__(self, client: NexoriousClient):
        self.client = client
    
    async def search_igdb(self, query: str, limit: int = 10):
        """Search IGDB for games"""
        response = await self.client.request(
            "POST", "/games/search/igdb",
            json={"query": query, "limit": limit}
        )
        return response.json()
    
    async def list_games(self, **filters):
        """List games in global database"""
        response = await self.client.request("GET", "/games", params=filters)
        return response.json()
    
    async def get_game(self, game_id: str):
        """Get game details from global database"""
        response = await self.client.request("GET", f"/games/{game_id}")
        return response.json()
    
    async def import_from_igdb(self, igdb_id: str, custom_overrides: dict = None):
        """Import game from IGDB to global database"""
        data = {"igdb_id": igdb_id}
        if custom_overrides:
            data["custom_overrides"] = custom_overrides
        response = await self.client.request("POST", "/games/igdb-import", json=data)
        return response.json()
    
    async def get_game_aliases(self, game_id: str):
        """Get aliases for a game"""
        response = await self.client.request("GET", f"/games/{game_id}/aliases")
        return response.json()


class UserGamesService:
    """Service for personal collection management (/api/user-games/*)"""
    def __init__(self, client: NexoriousClient):
        self.client = client
    
    async def add_to_collection(self, game_id: str, **user_game_data):
        """Add game to personal collection"""
        data = {"game_id": game_id, **user_game_data}
        response = await self.client.request("POST", "/user-games", json=data)
        return response.json()
    
    async def list_collection(self, **filters):
        """List personal game collection"""
        response = await self.client.request("GET", "/user-games", params=filters)
        return response.json()
    
    async def get_user_game(self, user_game_id: str):
        """Get collection item details"""
        response = await self.client.request("GET", f"/user-games/{user_game_id}")
        return response.json()
    
    async def update_user_game(self, user_game_id: str, **updates):
        """Update collection item"""
        response = await self.client.request("PUT", f"/user-games/{user_game_id}", json=updates)
        return response.json()
    
    async def update_progress(self, user_game_id: str, **progress_data):
        """Update game progress"""
        response = await self.client.request("PUT", f"/user-games/{user_game_id}/progress", json=progress_data)
        return response.json()
    
    async def remove_from_collection(self, user_game_id: str):
        """Remove game from collection"""
        response = await self.client.request("DELETE", f"/user-games/{user_game_id}")
        return response.json()
    
    async def get_collection_stats(self):
        """Get collection statistics"""
        response = await self.client.request("GET", "/user-games/stats")
        return response.json()
```

### Error Handling Pattern

```python
async def handle_api_response(response: httpx.Response):
    """Handle API response with proper error handling"""
    if response.status_code >= 400:
        try:
            error_data = response.json()
            error_msg = error_data.get('error', f'HTTP {response.status_code}')
        except json.JSONDecodeError:
            error_msg = f'HTTP {response.status_code}'
        
        if response.status_code == 401:
            raise AuthenticationError(error_msg)
        elif response.status_code == 403:
            raise APIError(f"Access denied: {error_msg}")
        # ... handle other status codes
    
    return response.json()
```

## Command Implementation Examples

### Games Command Implementation (Global Database)

```python
class GamesCommand(BaseCommand):
    def add_parser(self, subparsers):
        parser = subparsers.add_parser('games', help='Global game database operations')
        games_sub = parser.add_subparsers(dest='games_action')
        
        # games search
        search_parser = games_sub.add_parser('search', help='Search IGDB for games')
        search_parser.add_argument('query', help='Search query')
        search_parser.add_argument('--limit', type=int, default=10, help='Max results')
        
        # games list
        list_parser = games_sub.add_parser('list', help='List games in global database')
        list_parser.add_argument('--page', type=int, default=1)
        list_parser.add_argument('--search', help='Search query')
        
        # games import
        import_parser = games_sub.add_parser('import', help='Import game from IGDB')
        import_parser.add_argument('igdb_id', help='IGDB ID')
        
        return parser
    
    async def execute(self, args):
        games_service = GamesService(self.client)
        
        if args.games_action == 'search':
            return await self._search_igdb(games_service, args)
        elif args.games_action == 'list':
            return await self._list_games(games_service, args)
        elif args.games_action == 'import':
            return await self._import_game(games_service, args)
        # ... handle other actions
    
    async def _search_igdb(self, service, args):
        try:
            result = await service.search_igdb(
                query=args.query,
                limit=args.limit
            )
            self._print_igdb_candidates_table(result['games'])
            return 0
        except Exception as e:
            print(f"Error: {e}")
            return 1
    
    async def _import_game(self, service, args):
        try:
            result = await service.import_from_igdb(args.igdb_id)
            print(f"Successfully imported game: {result['title']} (ID: {result['id']})")
            return 0
        except Exception as e:
            print(f"Error: {e}")
            return 1


### UserGames Command Implementation (Personal Collection)

```python
class UserGamesCommand(BaseCommand):
    def add_parser(self, subparsers):
        parser = subparsers.add_parser('usergames', help='Personal collection management')
        usergames_sub = parser.add_subparsers(dest='usergames_action')
        
        # usergames add
        add_parser = usergames_sub.add_parser('add', help='Add game to collection')
        add_parser.add_argument('game_id', help='Game ID from global database')
        add_parser.add_argument('--status', help='Ownership status')
        add_parser.add_argument('--platform', help='Platform name')
        
        # usergames list
        list_parser = usergames_sub.add_parser('list', help='List personal collection')
        list_parser.add_argument('--page', type=int, default=1)
        list_parser.add_argument('--search', help='Search query')
        list_parser.add_argument('--filter', help='Filter by status')
        
        # usergames update
        update_parser = usergames_sub.add_parser('update', help='Update collection item')
        update_parser.add_argument('user_game_id', help='User game ID')
        update_parser.add_argument('--status', help='Play status')
        update_parser.add_argument('--rating', type=float, help='Personal rating')
        
        return parser
    
    async def execute(self, args):
        usergames_service = UserGamesService(self.client)
        
        if args.usergames_action == 'add':
            return await self._add_to_collection(usergames_service, args)
        elif args.usergames_action == 'list':
            return await self._list_collection(usergames_service, args)
        elif args.usergames_action == 'update':
            return await self._update_user_game(usergames_service, args)
        # ... handle other actions
    
    async def _add_to_collection(self, service, args):
        try:
            user_game_data = {"ownership_status": args.status} if args.status else {}
            if args.platform:
                user_game_data["platforms"] = [{"platform_name": args.platform}]
            
            result = await service.add_to_collection(args.game_id, **user_game_data)
            print(f"Added game to collection (ID: {result['id']})")
            return 0
        except Exception as e:
            print(f"Error: {e}")
            return 1
    
    async def _list_collection(self, service, args):
        try:
            filters = {"page": args.page}
            if args.search:
                filters["search"] = args.search
            if args.filter:
                # Parse filter like "status=owned"
                key, value = args.filter.split("=", 1)
                filters[key] = value
                
            result = await service.list_collection(**filters)
            self._print_collection_table(result['user_games'])
            return 0
        except Exception as e:
            print(f"Error: {e}")
            return 1
```

### Import Command with Platform Resolution

```python
class ImportCommand(BaseCommand):
    async def _handle_platform_resolution(self, import_job_id):
        """Handle platform resolution workflow"""
        # Get pending resolutions
        resolutions = await self.import_service.get_pending_resolutions()
        
        for resolution in resolutions:
            # Get suggestions
            suggestions = await self.import_service.get_platform_suggestions(
                resolution['unknown_platform']
            )
            
            # Present options to user
            choice = self._prompt_resolution_choice(suggestions)
            
            # Apply resolution
            await self.import_service.resolve_platform(
                resolution['import_id'],
                choice['platform_id'],
                choice.get('storefront_id')
            )
```

## Main Entry Point Implementation

```python
async def main():
    parser = argparse.ArgumentParser(prog='nexcmd')
    
    # Add global arguments with environment defaults
    parser.add_argument('--server', 
                       default=os.environ.get('NEXORIOUS_SERVER'))
    # ... more global args
    
    args = parser.parse_args()
    
    # Initialize configuration with precedence handling
    config = ConfigManager(config_dir=args.config)
    
    # Apply environment and CLI overrides
    # Initialize managers
    # Execute commands
```

## Testing Implementation

### Unit Test Pattern

```python
class TestConfigManager:
    def setup_method(self):
        self.temp_dir = tempfile.mkdtemp()
        self.config = ConfigManager(config_dir=self.temp_dir)
    
    def test_xdg_directory_creation(self):
        """Test XDG directory structure creation"""
        assert self.config.config_dir.exists()
        assert self.config.auth_dir.exists()
        # Verify permissions
        assert oct(self.config.auth_dir.stat().st_mode)[-3:] == '700'
```

### Integration Test Pattern

```python
@pytest.mark.asyncio
async def test_games_list_command():
    """Test games list command integration"""
    # Setup mock responses
    # Create command instance
    # Execute command
    # Verify API calls and output
```

## Development Dependencies

```toml
# cli/pyproject.toml
[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[project]
name = "nexorious-cli"
version = "1.0.0"
description = "Command-line interface for Nexorious Game Collection Management"
authors = [{name = "Your Name", email = "your.email@example.com"}]
license = {text = "MIT"}
readme = "README.md"
requires-python = ">=3.9"
dependencies = [
    "httpx>=0.28.0",
    "toml>=0.10.2", 
    "rich>=13.0.0",
    "tabulate>=0.9.0",
    "python-dateutil>=2.8.0",
    "pydantic>=2.0.0",
]

[project.scripts]
nexcmd = "nexorious_cli.main:main"

[project.urls]
Homepage = "https://github.com/your-org/nexorious"
Repository = "https://github.com/your-org/nexorious"
Issues = "https://github.com/your-org/nexorious/issues"
```

## Best Practices

### Error Handling
- Use specific exception types
- Provide helpful error messages
- Log errors appropriately
- Return appropriate exit codes

### User Experience
- Show progress for long operations
- Confirm destructive operations
- Provide helpful error recovery suggestions
- Support both verbose and quiet modes

### Security
- Never log sensitive data
- Validate all inputs
- Use secure file permissions
- Handle authentication failures gracefully

### Performance
- Use async operations
- Implement connection pooling
- Optimize API calls

This implementation guide provides the foundation for building a robust, secure, and user-friendly CLI client that meets the specification requirements.