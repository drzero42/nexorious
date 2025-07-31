"""
Merge Strategies for Darkadia Import

This module implements different strategies for resolving conflicts
when importing games that already exist in the user's collection.
"""

import asyncio
from abc import ABC, abstractmethod
from typing import Dict, Any, List, Optional, Tuple
from datetime import datetime
import json
from pathlib import Path
import hashlib

from rich.console import Console
from rich.table import Table
from rich.prompt import Prompt, Confirm
from rich.progress import Progress, SpinnerColumn, TextColumn, BarColumn

from .api_client import NexoriousAPIClient, APIException
from .mapper import DarkadiaDataMapper

console = Console()


class MergeStrategy(ABC):
    """Abstract base class for merge strategies."""
    
    def __init__(self, api_client: NexoriousAPIClient, dry_run: bool = False):
        self.api_client = api_client
        self.dry_run = dry_run
        self.mapper = DarkadiaDataMapper()
        self.results = {
            'total_processed': 0,
            'new_games': 0,
            'updated_games': 0,
            'skipped_games': 0,
            'errors': 0,
            'error_details': []
        }
    
    @abstractmethod
    async def process_games(self, games: List[Dict[str, Any]], user_id: str, 
                           batch_size: int = 10) -> Dict[str, Any]:
        """Process a list of games according to the merge strategy."""
        pass
    
    @abstractmethod
    async def handle_conflict(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any]) -> Dict[str, Any]:
        """Handle conflict between existing game and CSV data."""
        pass
    
    async def find_existing_game(self, game_title: str, user_id: str) -> Optional[Dict[str, Any]]:
        """
        Find existing game in user's collection with enhanced matching logic.
        
        Uses a tiered approach:
        1. Exact title match (case insensitive)
        2. High fuzzy threshold match (0.95)
        3. Medium fuzzy threshold match (0.85)
        """
        try:
            normalized_title = game_title.strip().lower()
            
            # Try exact match first with high fuzzy threshold
            search_results = await self.api_client.search_games(game_title, fuzzy_threshold=0.95)
            
            if search_results:
                # Look for exact title match first
                for game in search_results:
                    if game.get('title', '').strip().lower() == normalized_title:
                        return game
                
                # Return the highest scoring match if no exact match
                return search_results[0]
            
            # Try with lower threshold if no high-confidence matches
            search_results = await self.api_client.search_games(game_title, fuzzy_threshold=0.85)
            
            if search_results:
                # Still prefer exact matches even at lower threshold
                for game in search_results:
                    if game.get('title', '').strip().lower() == normalized_title:
                        return game
                
                # Return first result only if confidence is reasonable
                return search_results[0]
            
            return None
            
        except APIException as e:
            console.print(f"[yellow]Error searching for game '{game_title}': {str(e)}[/yellow]")
            return None
    
    def _record_error(self, error_msg: str, game_title: str = ""):
        """Record an error for reporting."""
        self.results['errors'] += 1
        error_detail = f"{game_title}: {error_msg}" if game_title else error_msg
        self.results['error_details'].append(error_detail)
    
    async def _add_new_platforms_only(self, existing_game: Dict[str, Any], new_platforms: List[Dict[str, Any]]):
        """Add only platforms that don't already exist for the game."""
        existing_platforms = existing_game.get('platforms', [])
        
        for platform in new_platforms:
            # Check if this platform/storefront combination already exists
            platform_exists = any(
                ep.get('platform_name') == platform.get('platform_name') and
                ep.get('storefront_name') == platform.get('storefront_name')
                for ep in existing_platforms
            )
            
            if not platform_exists:
                try:
                    await self.api_client.add_platform_to_user_game(existing_game['id'], platform)
                except APIException as e:
                    self._record_error(f"Failed to add platform {platform.get('platform_name', 'Unknown')}: {str(e)}", 
                                     existing_game.get('title', 'Unknown'))
    
    def _get_new_platforms_to_add(self, existing_game: Dict[str, Any], new_platforms: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Get list of platforms that need to be added (filtering out duplicates)."""
        existing_platforms = existing_game.get('platforms', [])
        platforms_to_add = []
        
        for platform in new_platforms:
            # Check if this platform/storefront combination already exists
            platform_exists = any(
                ep.get('platform_name') == platform.get('platform_name') and
                ep.get('storefront_name') == platform.get('storefront_name')
                for ep in existing_platforms
            )
            
            if not platform_exists:
                platforms_to_add.append(platform)
        
        return platforms_to_add


class InteractiveMerger(MergeStrategy):
    """Interactive merge strategy - asks user for conflict resolution."""
    
    def __init__(self, console: Console, api_client: NexoriousAPIClient, dry_run: bool = False):
        super().__init__(api_client, dry_run)
        self.console = console
        self.batch_decisions = {}  # Cache decisions for similar conflicts
        self.decision_cache_file = Path.home() / '.nexorious' / 'import_decisions.json'
        self.persistent_decisions = {}  # Loaded from file
        self._load_persistent_decisions()
    
    def _load_persistent_decisions(self):
        """Load previously saved decisions from disk."""
        try:
            if self.decision_cache_file.exists():
                with open(self.decision_cache_file, 'r') as f:
                    self.persistent_decisions = json.load(f)
                console.print(f"[cyan]Loaded {len(self.persistent_decisions)} cached decisions[/cyan]")
        except Exception as e:
            console.print(f"[yellow]Could not load decision cache: {e}[/yellow]")
            self.persistent_decisions = {}
    
    def _save_persistent_decisions(self):
        """Save decisions to disk for future runs."""
        try:
            self.decision_cache_file.parent.mkdir(parents=True, exist_ok=True)
            with open(self.decision_cache_file, 'w') as f:
                json.dump(self.persistent_decisions, f, indent=2)
        except Exception as e:
            console.print(f"[yellow]Could not save decision cache: {e}[/yellow]")
    
    def _create_conflict_signature(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any]) -> str:
        """Create a unique signature for a conflict to enable caching."""
        # Create a hash based on the game title and the conflicting data
        conflict_data = {
            'title': existing_game.get('title', '').lower().strip(),
            'existing_rating': existing_game.get('personal_rating'),
            'existing_status': existing_game.get('play_status'),
            'existing_loved': existing_game.get('is_loved'),
            'csv_rating': csv_game.get('personal_rating'),
            'csv_status': csv_game.get('play_status'),
            'csv_loved': csv_game.get('is_loved'),
        }
        
        # Create hash from the conflict signature
        conflict_json = json.dumps(conflict_data, sort_keys=True)
        return hashlib.md5(conflict_json.encode()).hexdigest()
    
    async def process_games(self, games: List[Dict[str, Any]], user_id: str, 
                           batch_size: int = 10) -> Dict[str, Any]:
        """Process games with interactive conflict resolution."""
        
        console.print(f"Starting interactive import of {len(games)} games...")
        
        if self.dry_run:
            console.print("[yellow]DRY RUN MODE - No changes will be made[/yellow]")
        
        for i, darkadia_game in enumerate(games):
            game_title = darkadia_game.get('Name', 'Unknown Game')
            console.print(f"\n[cyan]Processing game {i+1} of {len(games)}: {game_title}[/cyan]")
            
            try:
                await self._process_single_game(darkadia_game, user_id)
                
            except KeyboardInterrupt:
                console.print("\n[yellow]Import cancelled by user[/yellow]")
                break
            except Exception as e:
                self._record_error(f"Unexpected error: {str(e)}", darkadia_game.get('Name', 'Unknown'))
        
        # Save any new decisions for future runs
        self._save_persistent_decisions()
        
        return self.results
    
    async def _process_single_game(self, darkadia_game: Dict[str, Any], user_id: str):
        """Process a single game with interactive resolution."""
        
        game_title = darkadia_game.get('Name', '').strip()
        if not game_title:
            self._record_error("Empty game title", "")
            return
        
        self.results['total_processed'] += 1
        
        # Convert to Nexorious format
        nexorious_game = self.mapper.convert_to_nexorious_game(darkadia_game)
        
        # Check if game already exists
        existing_game = await self.find_existing_game(game_title, user_id)
        
        if existing_game:
            # Conflict detected - ask user for resolution
            resolution = await self.handle_conflict(existing_game, nexorious_game)
            
            if resolution['action'] == 'skip':
                self.results['skipped_games'] += 1
                return
            elif resolution['action'] == 'update':
                if not self.dry_run:
                    try:
                        await self.api_client.update_user_game(existing_game['id'], resolution['data'])
                        
                        # Add only new platforms (check for duplicates)
                        await self._add_new_platforms_only(existing_game, nexorious_game.get('platforms', []))
                        
                        self.results['updated_games'] += 1
                        console.print(f"[green]✓ Updated: {game_title}[/green]")
                    except APIException as e:
                        self._record_error(f"Failed to update: {str(e)}", game_title)
                else:
                    console.print(f"[blue]DRY RUN: Would update {game_title}[/blue]")
                    self.results['updated_games'] += 1
        else:
            # New game - create it
            if not self.dry_run:
                try:
                    await self.api_client.create_user_game(user_id, nexorious_game)
                    self.results['new_games'] += 1
                    console.print(f"[green]✓ Added: {game_title}[/green]")
                except APIException as e:
                    self._record_error(f"Failed to create: {str(e)}", game_title)
            else:
                console.print(f"[blue]DRY RUN: Would add {game_title}[/blue]")
                self.results['new_games'] += 1
    
    async def handle_conflict(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any]) -> Dict[str, Any]:
        """Handle conflict with interactive prompts."""
        
        game_title = existing_game.get('title', csv_game.get('title', 'Unknown Game'))
        
        # Check for cached decision first
        conflict_signature = self._create_conflict_signature(existing_game, csv_game)
        if conflict_signature in self.persistent_decisions:
            cached_decision = self.persistent_decisions[conflict_signature]
            console.print(f"[cyan]Using cached decision for '{game_title}': {cached_decision['choice_description']}[/cyan]")
            return self._apply_cached_decision(existing_game, csv_game, cached_decision)
        
        # Check if we have a batch decision for this type of conflict
        conflict_type = self._classify_conflict(existing_game, csv_game)
        if conflict_type in self.batch_decisions:
            return self._apply_batch_decision(existing_game, csv_game, self.batch_decisions[conflict_type])
        
        # Show conflict details
        console.print(f"\n[bold yellow]⚠️  Game Conflict: {game_title}[/bold yellow]")
        
        # Create comparison table
        table = Table(title="Data Comparison")
        table.add_column("Field", style="cyan")
        table.add_column("CSV Data", style="yellow")
        table.add_column("Existing Data", style="green")
        
        # Compare key fields - only show fields that actually differ
        all_comparisons = [
            ('Rating', existing_game.get('personal_rating'), csv_game.get('personal_rating')),
            ('Play Status', existing_game.get('play_status'), csv_game.get('play_status')),
            ('Loved', existing_game.get('is_loved'), csv_game.get('is_loved')),
            ('Hours', existing_game.get('hours_played', 0), csv_game.get('hours_played', 0)),
            ('Ownership Status', existing_game.get('ownership_status'), csv_game.get('ownership_status')),
            ('Acquired Date', existing_game.get('acquired_date'), csv_game.get('acquired_date')),
            ('Notes', (existing_game.get('personal_notes') or '')[:50] + '...' if len(existing_game.get('personal_notes', '')) > 50 else existing_game.get('personal_notes', ''),
                     (csv_game.get('personal_notes') or '')[:50] + '...' if len(csv_game.get('personal_notes', '')) > 50 else csv_game.get('personal_notes', ''))
        ]
        
        # Filter to only show differences
        differences = []
        for field, existing, csv_val in all_comparisons:
            if existing != csv_val:
                differences.append((field, existing, csv_val))
        
        if differences:
            for field, existing, csv_val in differences:
                table.add_row(field, str(csv_val) if csv_val is not None else 'None',
                             str(existing) if existing is not None else 'None')
        else:
            table.add_row("No Data Differences", "All fields match", "All fields match")
        
        console.print(table)
        
        # Show platform/storefront differences
        existing_platforms = existing_game.get('platforms', [])
        csv_platforms = csv_game.get('platforms', [])
        
        if csv_platforms:
            console.print(f"\n[bold cyan]Platform Changes:[/bold cyan]")
            console.print(f"  Current platforms: {len(existing_platforms)}")
            for platform in existing_platforms:
                console.print(f"    • {platform.get('platform_name', 'Unknown')} ({platform.get('storefront_name', 'Unknown')})")
            
            console.print(f"  Adding from CSV: {len(csv_platforms)}")
            for platform in csv_platforms:
                console.print(f"    • {platform.get('platform_name', 'Unknown')} ({platform.get('storefront_name', 'Unknown')})")
        
        # Show resolution options
        console.print("\n[bold]Resolution Options:[/bold]")
        console.print("  1) Keep existing data")
        console.print("  2) Use CSV data")  
        console.print("  3) Merge intelligently (combine best of both)")
        console.print("  4) Skip this game")
        console.print("  5) Apply to all similar conflicts")
        
        choice = await asyncio.to_thread(Prompt.ask, "Choice", choices=['1', '2', '3', '4', '5'], default='1')
        
        # Create decision record for caching
        choice_descriptions = {
            '1': 'Keep existing data',
            '2': 'Use CSV data',
            '3': 'Merge intelligently',
            '4': 'Skip this game',
            '5': 'Apply to all similar conflicts'
        }
        
        decision_record = {
            'choice': choice,
            'choice_description': choice_descriptions[choice],
            'timestamp': datetime.now().isoformat()
        }
        
        if choice == '1':
            # Save decision to cache
            self.persistent_decisions[conflict_signature] = decision_record
            return {'action': 'skip'}  # Keep existing, don't update
        elif choice == '2':
            # Save decision to cache
            self.persistent_decisions[conflict_signature] = decision_record
            return {'action': 'update', 'data': csv_game}
        elif choice == '3':
            # Save decision to cache
            self.persistent_decisions[conflict_signature] = decision_record
            merged_data = self._merge_intelligently(existing_game, csv_game)
            return {'action': 'update', 'data': merged_data}
        elif choice == '4':
            # Save decision to cache
            self.persistent_decisions[conflict_signature] = decision_record
            return {'action': 'skip'}
        elif choice == '5':
            batch_choice = await asyncio.to_thread(Prompt.ask, "Apply which strategy to similar conflicts?", 
                                    choices=['1', '2', '3'], default='1')
            self.batch_decisions[conflict_type] = batch_choice
            # Update decision record with batch choice details
            decision_record['batch_choice'] = batch_choice
            decision_record['choice_description'] = f"Apply '{choice_descriptions[batch_choice]}' to similar conflicts"
            self.persistent_decisions[conflict_signature] = decision_record
            return self._apply_batch_decision(existing_game, csv_game, batch_choice)
        
        return {'action': 'skip'}
    
    def _apply_cached_decision(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any], cached_decision: Dict[str, Any]) -> Dict[str, Any]:
        """Apply a previously cached decision."""
        choice = cached_decision.get('choice', '1')
        
        if choice == '1':
            return {'action': 'skip'}  # Keep existing, don't update
        elif choice == '2':
            return {'action': 'update', 'data': csv_game}
        elif choice == '3':
            merged_data = self._merge_intelligently(existing_game, csv_game)
            return {'action': 'update', 'data': merged_data}
        elif choice == '4':
            return {'action': 'skip'}
        elif choice == '5':
            # Use the batch choice from the cached decision
            batch_choice = cached_decision.get('batch_choice', '1')
            return self._apply_batch_decision(existing_game, csv_game, batch_choice)
        
        return {'action': 'skip'}
    
    def _classify_conflict(self, existing: Dict[str, Any], csv: Dict[str, Any]) -> str:
        """Classify the type of conflict for batch decisions."""
        conflicts = []
        
        if existing.get('personal_rating') != csv.get('personal_rating'):
            conflicts.append('rating')
        if existing.get('play_status') != csv.get('play_status'):
            conflicts.append('status')
        if existing.get('is_loved') != csv.get('is_loved'):
            conflicts.append('loved')
        if existing.get('personal_notes', '').strip() != csv.get('personal_notes', '').strip():
            conflicts.append('notes')
        
        return '_'.join(sorted(conflicts)) if conflicts else 'no_conflict'
    
    def _apply_batch_decision(self, existing: Dict[str, Any], csv: Dict[str, Any], decision: str) -> Dict[str, Any]:
        """Apply a batch decision to resolve conflict."""
        if decision == '1':
            return {'action': 'skip'}
        elif decision == '2':
            return {'action': 'update', 'data': csv}
        elif decision == '3':
            merged_data = self._merge_intelligently(existing, csv)
            return {'action': 'update', 'data': merged_data}
        
        return {'action': 'skip'}
    
    def _merge_intelligently(self, existing: Dict[str, Any], csv: Dict[str, Any]) -> Dict[str, Any]:
        """Merge data intelligently using best practices."""
        merged = existing.copy()
        
        # Use higher rating
        existing_rating = existing.get('personal_rating') or 0
        csv_rating = csv.get('personal_rating') or 0
        if csv_rating > existing_rating:
            merged['personal_rating'] = csv_rating
        
        # Use higher play status (based on progression)
        status_order = ['not_started', 'in_progress', 'completed', 'mastered', 'dominated']
        existing_status = existing.get('play_status', 'not_started')
        csv_status = csv.get('play_status', 'not_started')
        
        if status_order.index(csv_status) > status_order.index(existing_status):
            merged['play_status'] = csv_status
        
        # Combine notes
        existing_notes = existing.get('personal_notes', '').strip()
        csv_notes = csv.get('personal_notes', '').strip()
        
        if existing_notes and csv_notes and existing_notes != csv_notes:
            merged['personal_notes'] = f"{existing_notes} | {csv_notes}"
        elif csv_notes and not existing_notes:
            merged['personal_notes'] = csv_notes
        
        # Use OR logic for loved status
        merged['is_loved'] = existing.get('is_loved', False) or csv.get('is_loved', False)
        
        # Use more recent acquired date
        existing_date = existing.get('acquired_date')
        csv_date = csv.get('acquired_date')
        if csv_date and (not existing_date or csv_date > existing_date):
            merged['acquired_date'] = csv_date
        
        return merged


class OverwriteMerger(MergeStrategy):
    """Overwrite merge strategy - CSV data always takes precedence."""
    
    async def process_games(self, games: List[Dict[str, Any]], user_id: str, 
                           batch_size: int = 10) -> Dict[str, Any]:
        """Process games with overwrite strategy."""
        
        console.print(f"Starting overwrite import of {len(games)} games...")
        
        with Progress(
            SpinnerColumn(), 
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            transient=False
        ) as progress:
            task = progress.add_task("Processing games...", total=len(games))
            
            # Set progress console for API client messages
            self.api_client.set_progress_console(progress.console)
            
            for darkadia_game in games:
                try:
                    await self._process_single_game(darkadia_game, user_id)
                except Exception as e:
                    self._record_error(f"Unexpected error: {str(e)}", darkadia_game.get('Name', 'Unknown'))
                finally:
                    progress.update(task, advance=1)
            
            # Reset to default console after progress tracking
            self.api_client.set_progress_console(None)
        
        return self.results
    
    async def _process_single_game(self, darkadia_game: Dict[str, Any], user_id: str):
        """Process a single game with overwrite strategy."""
        
        game_title = darkadia_game.get('Name', '').strip()
        if not game_title:
            self._record_error("Empty game title", "")
            return
        
        self.results['total_processed'] += 1
        
        # Convert to Nexorious format
        nexorious_game = self.mapper.convert_to_nexorious_game(darkadia_game)
        
        # Check if game already exists
        existing_game = await self.find_existing_game(game_title, user_id)
        
        if existing_game:
            # Update with CSV data
            resolution = await self.handle_conflict(existing_game, nexorious_game)
            
            if not self.dry_run:
                try:
                    await self.api_client.update_user_game(existing_game['id'], resolution['data'])
                    
                    # Add only new platforms (check for duplicates)
                    await self._add_new_platforms_only(existing_game, nexorious_game.get('platforms', []))
                    
                    self.results['updated_games'] += 1
                except APIException as e:
                    self._record_error(f"Failed to update: {str(e)}", game_title)
            else:
                self.results['updated_games'] += 1
        else:
            # New game - create it
            if not self.dry_run:
                try:
                    await self.api_client.create_user_game(user_id, nexorious_game)
                    self.results['new_games'] += 1
                except APIException as e:
                    self._record_error(f"Failed to create: {str(e)}", game_title)
            else:
                self.results['new_games'] += 1
    
    async def handle_conflict(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any]) -> Dict[str, Any]:
        """Always use CSV data for conflicts."""
        return {'action': 'update', 'data': csv_game}


class PreserveMerger(MergeStrategy):
    """Preserve merge strategy - never overwrite existing data."""
    
    async def process_games(self, games: List[Dict[str, Any]], user_id: str, 
                           batch_size: int = 10) -> Dict[str, Any]:
        """Process games with preserve strategy."""
        
        console.print(f"Starting preserve import of {len(games)} games...")
        
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            transient=False
        ) as progress:
            task = progress.add_task("Processing games...", total=len(games))
            
            # Set progress console for API client messages
            self.api_client.set_progress_console(progress.console)
            
            for darkadia_game in games:
                try:
                    await self._process_single_game(darkadia_game, user_id)
                except Exception as e:
                    self._record_error(f"Unexpected error: {str(e)}", darkadia_game.get('Name', 'Unknown'))
                finally:
                    progress.update(task, advance=1)
            
            # Reset to default console after progress tracking
            self.api_client.set_progress_console(None)
        
        return self.results
    
    async def _process_single_game(self, darkadia_game: Dict[str, Any], user_id: str):
        """Process a single game with preserve strategy."""
        
        game_title = darkadia_game.get('Name', '').strip()
        if not game_title:
            self._record_error("Empty game title", "")
            return
        
        self.results['total_processed'] += 1
        
        # Convert to Nexorious format
        nexorious_game = self.mapper.convert_to_nexorious_game(darkadia_game)
        
        # Check if game already exists
        existing_game = await self.find_existing_game(game_title, user_id)
        
        if existing_game:
            # Only add new platforms, don't update game data
            platforms_to_add = self._get_new_platforms_to_add(existing_game, nexorious_game.get('platforms', []))
            
            if platforms_to_add:
                if not self.dry_run:
                    await self._add_new_platforms_only(existing_game, platforms_to_add)
                self.results['updated_games'] += 1
            else:
                self.results['skipped_games'] += 1
        else:
            # New game - create it
            if not self.dry_run:
                try:
                    await self.api_client.create_user_game(user_id, nexorious_game)
                    self.results['new_games'] += 1
                except APIException as e:
                    self._record_error(f"Failed to create: {str(e)}", game_title)
            else:
                self.results['new_games'] += 1
    
    async def handle_conflict(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any]) -> Dict[str, Any]:
        """Never overwrite - always preserve existing data."""
        return {'action': 'skip'}