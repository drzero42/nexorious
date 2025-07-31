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
                           batch_size: int = 10, resume_file: Optional[Path] = None) -> Dict[str, Any]:
        """Process a list of games according to the merge strategy."""
        pass
    
    @abstractmethod
    async def handle_conflict(self, existing_game: Dict[str, Any], csv_game: Dict[str, Any]) -> Dict[str, Any]:
        """Handle conflict between existing game and CSV data."""
        pass
    
    async def find_existing_game(self, game_title: str, user_id: str) -> Optional[Dict[str, Any]]:
        """Find existing game in user's collection."""
        try:
            search_results = await self.api_client.search_games(game_title, fuzzy_threshold=0.85)
            
            if search_results:
                # Return first match for now - could be made more sophisticated
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


class InteractiveMerger(MergeStrategy):
    """Interactive merge strategy - asks user for conflict resolution."""
    
    def __init__(self, console: Console, api_client: NexoriousAPIClient, dry_run: bool = False):
        super().__init__(api_client, dry_run)
        self.console = console
        self.batch_decisions = {}  # Cache decisions for similar conflicts
    
    async def process_games(self, games: List[Dict[str, Any]], user_id: str, 
                           batch_size: int = 10, resume_file: Optional[Path] = None) -> Dict[str, Any]:
        """Process games with interactive conflict resolution."""
        
        console.print(f"Starting interactive import of {len(games)} games...")
        
        if self.dry_run:
            console.print("[yellow]DRY RUN MODE - No changes will be made[/yellow]")
        
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            transient=False
        ) as progress:
            task = progress.add_task("Processing games...", total=len(games))
            
            for i, darkadia_game in enumerate(games):
                try:
                    await self._process_single_game(darkadia_game, user_id)
                    progress.update(task, advance=1)
                    
                except KeyboardInterrupt:
                    console.print("\n[yellow]Import cancelled by user[/yellow]")
                    break
                except Exception as e:
                    self._record_error(f"Unexpected error: {str(e)}", darkadia_game.get('Name', 'Unknown'))
                    progress.update(task, advance=1)
        
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
                        # Add any new platforms
                        for platform in nexorious_game.get('platforms', []):
                            await self.api_client.add_platform_to_user_game(existing_game['id'], platform)
                        
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
        
        game_title = existing_game.get('title', 'Unknown Game')
        
        # Check if we have a batch decision for this type of conflict
        conflict_type = self._classify_conflict(existing_game, csv_game)
        if conflict_type in self.batch_decisions:
            return self._apply_batch_decision(existing_game, csv_game, self.batch_decisions[conflict_type])
        
        # Show conflict details
        console.print(f"\n[bold yellow]⚠️  Game Conflict: {game_title}[/bold yellow]")
        
        # Create comparison table
        table = Table(title="Data Comparison")
        table.add_column("Field", style="cyan")
        table.add_column("Existing Data", style="green")
        table.add_column("CSV Data", style="yellow")
        
        # Compare key fields
        comparisons = [
            ('Rating', existing_game.get('personal_rating'), csv_game.get('personal_rating')),
            ('Play Status', existing_game.get('play_status'), csv_game.get('play_status')),
            ('Loved', existing_game.get('is_loved'), csv_game.get('is_loved')),
            ('Hours', existing_game.get('hours_played', 0), csv_game.get('hours_played', 0)),
            ('Notes', (existing_game.get('personal_notes') or '')[:50] + '...' if len(existing_game.get('personal_notes', '')) > 50 else existing_game.get('personal_notes', ''),
                     (csv_game.get('personal_notes') or '')[:50] + '...' if len(csv_game.get('personal_notes', '')) > 50 else csv_game.get('personal_notes', ''))
        ]
        
        for field, existing, csv_val in comparisons:
            table.add_row(field, str(existing) if existing is not None else 'None', 
                         str(csv_val) if csv_val is not None else 'None')
        
        console.print(table)
        
        # Show resolution options
        console.print("\n[bold]Resolution Options:[/bold]")
        console.print("  1) Keep existing data")
        console.print("  2) Use CSV data")  
        console.print("  3) Merge intelligently (combine best of both)")
        console.print("  4) Skip this game")
        console.print("  5) Apply to all similar conflicts")
        
        choice = Prompt.ask("Choice", choices=['1', '2', '3', '4', '5'], default='1')
        
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
            batch_choice = Prompt.ask("Apply which strategy to similar conflicts?", 
                                    choices=['1', '2', '3'], default='1')
            self.batch_decisions[conflict_type] = batch_choice
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
                           batch_size: int = 10, resume_file: Optional[Path] = None) -> Dict[str, Any]:
        """Process games with overwrite strategy."""
        
        console.print(f"Starting overwrite import of {len(games)} games...")
        
        with Progress(
            SpinnerColumn(), 
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            transient=False
        ) as progress:
            task = progress.add_task("Processing games...", total=len(games))
            
            for darkadia_game in games:
                try:
                    await self._process_single_game(darkadia_game, user_id)
                except Exception as e:
                    self._record_error(f"Unexpected error: {str(e)}", darkadia_game.get('Name', 'Unknown'))
                finally:
                    progress.update(task, advance=1)
        
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
                    # Add any new platforms
                    for platform in nexorious_game.get('platforms', []):
                        await self.api_client.add_platform_to_user_game(existing_game['id'], platform)
                    
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
                           batch_size: int = 10, resume_file: Optional[Path] = None) -> Dict[str, Any]:
        """Process games with preserve strategy."""
        
        console.print(f"Starting preserve import of {len(games)} games...")
        
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            transient=False
        ) as progress:
            task = progress.add_task("Processing games...", total=len(games))
            
            for darkadia_game in games:
                try:
                    await self._process_single_game(darkadia_game, user_id)
                except Exception as e:
                    self._record_error(f"Unexpected error: {str(e)}", darkadia_game.get('Name', 'Unknown'))
                finally:
                    progress.update(task, advance=1)
        
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
            new_platforms_added = False
            
            if not self.dry_run:
                for platform in nexorious_game.get('platforms', []):
                    # Check if this platform is already associated
                    existing_platforms = existing_game.get('platforms', [])
                    platform_exists = any(
                        ep.get('platform_name') == platform['platform_name'] and
                        ep.get('storefront_name') == platform['storefront_name']
                        for ep in existing_platforms
                    )
                    
                    if not platform_exists:
                        try:
                            await self.api_client.add_platform_to_user_game(existing_game['id'], platform)
                            new_platforms_added = True
                        except APIException as e:
                            self._record_error(f"Failed to add platform: {str(e)}", game_title)
            
            if new_platforms_added:
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