#!/usr/bin/env python3
"""
Darkadia CSV Import Script for Nexorious

This script imports game collections from Darkadia CSV export files into
the Nexorious game collection management system.

Usage:
    python import_darkadia_csv.py [CSV_FILE] [OPTIONS]

The script supports three merge strategies for handling conflicts:
- Interactive: Ask user for resolution (default)
- Overwrite: CSV data takes precedence
- Preserve: Never overwrite existing data
"""

import sys
import asyncio
from pathlib import Path
from typing import Optional

import click
from rich.console import Console
from rich.table import Table

# Add the parent directory to the path so we can import nexorious modules
sys.path.insert(0, str(Path(__file__).parent.parent))

from scripts.darkadia.parser import DarkadiaCSVParser
from scripts.darkadia.api_client import NexoriousAPIClient
from scripts.darkadia.merge_strategies import MergeStrategy, InteractiveMerger, OverwriteMerger, PreserveMerger

console = Console()


@click.command()
@click.argument('csv_file', type=click.Path(exists=True, dir_okay=False, path_type=Path))
@click.option('--user-id', required=True, help='User ID for import')
@click.option('--api-base', default='http://localhost:8000', help='Backend API base URL')
@click.option('--interactive', 'merge_strategy', flag_value='interactive', default=True,
              help='Pause and ask user for conflict resolution (default)')
@click.option('--overwrite', 'merge_strategy', flag_value='overwrite',
              help='Always use CSV data, overwrite existing data')
@click.option('--preserve', 'merge_strategy', flag_value='preserve',
              help='Never overwrite, only add new games/platforms')
@click.option('--dry-run', is_flag=True, help='Preview changes without making them')
@click.option('--batch-size', default=10, help='Process N games at a time')
@click.option('--auth-token', help='API authentication token')
@click.option('--username', help='Username for authentication (if no token provided)')
@click.option('--password', help='Password for authentication (if no token provided)')
@click.option('--verbose', is_flag=True, help='Enable verbose logging')
@click.option('--resume', type=click.Path(path_type=Path), help='Resume from saved progress file')
def import_csv(
    csv_file: Path,
    user_id: str,
    api_base: str,
    merge_strategy: str,
    dry_run: bool,
    batch_size: int,
    auth_token: Optional[str],
    username: Optional[str],
    password: Optional[str],
    verbose: bool,
    resume: Optional[Path]
):
    """Import games from Darkadia CSV export file."""
    
    console.print("[bold green]Darkadia CSV Import Tool[/bold green]")
    console.print(f"CSV File: {csv_file}")
    console.print(f"Target User: {user_id}")
    console.print(f"Merge Strategy: {merge_strategy}")
    
    if dry_run:
        console.print("[yellow]DRY RUN MODE - No changes will be made[/yellow]")
    
    try:
        # Run the async import process
        asyncio.run(run_import(
            csv_file=csv_file,
            user_id=user_id,
            api_base=api_base,
            merge_strategy=merge_strategy,
            dry_run=dry_run,
            batch_size=batch_size,
            auth_token=auth_token,
            username=username,
            password=password,
            verbose=verbose,
            resume=resume
        ))
        
    except KeyboardInterrupt:
        console.print("\n[yellow]Import cancelled by user[/yellow]")
        sys.exit(1)
    except Exception as e:
        console.print(f"\n[red]Import failed: {str(e)}[/red]")
        if verbose:
            import traceback
            console.print(traceback.format_exc())
        sys.exit(1)


async def run_import(
    csv_file: Path,
    user_id: str,
    api_base: str,
    merge_strategy: str,
    dry_run: bool,
    batch_size: int,
    auth_token: Optional[str],
    username: Optional[str],
    password: Optional[str],
    verbose: bool,
    resume: Optional[Path]
):
    """Run the main import process."""
    
    # Phase 1: Parse CSV file
    console.print("\n[cyan]Phase 1: Parsing CSV file...[/cyan]")
    parser = DarkadiaCSVParser(verbose=verbose)
    games_data = await parser.parse_csv(csv_file)
    console.print(f"✓ Found {len(games_data)} games in CSV")
    
    # Phase 2: Group duplicates
    console.print("\n[cyan]Phase 2: Grouping duplicates...[/cyan]")
    unique_games = await parser.group_duplicates(games_data)
    console.print(f"✓ Identified {len(unique_games)} unique games")
    
    # Phase 3: Setup API client
    console.print("\n[cyan]Phase 3: Connecting to API...[/cyan]")
    api_client = NexoriousAPIClient(api_base)
    
    if auth_token:
        api_client.set_token(auth_token)
    elif username and password:
        await api_client.authenticate(username, password)
    else:
        console.print("[red]Error: Must provide either --auth-token or --username/--password[/red]")
        return
    
    console.print("✓ API connection established")
    
    # Phase 4: Setup merge strategy
    console.print(f"\n[cyan]Phase 4: Setting up {merge_strategy} merge strategy...[/cyan]")
    
    if merge_strategy == 'interactive':
        merger = InteractiveMerger(console, api_client, dry_run)
    elif merge_strategy == 'overwrite':
        merger = OverwriteMerger(api_client, dry_run)
    elif merge_strategy == 'preserve':
        merger = PreserveMerger(api_client, dry_run)
    else:
        raise ValueError(f"Unknown merge strategy: {merge_strategy}")
    
    console.print("✓ Merge strategy configured")
    
    # Phase 5: Process games
    console.print(f"\n[cyan]Phase 5: Processing {len(unique_games)} games...[/cyan]")
    
    results = await merger.process_games(
        unique_games, 
        user_id, 
        batch_size=batch_size, 
        resume_file=resume
    )
    
    # Phase 6: Generate report
    console.print("\n[cyan]Phase 6: Generating report...[/cyan]")
    generate_final_report(results, csv_file, merge_strategy)
    
    console.print("\n[bold green]Import completed successfully![/bold green]")


def generate_final_report(results: dict, csv_file: Path, merge_strategy: str):
    """Generate and display the final import report."""
    
    console.print("\n[bold blue]Import Summary Report[/bold blue]")
    
    # Create summary table
    table = Table(title="Import Results")
    table.add_column("Metric", style="cyan")
    table.add_column("Count", justify="right", style="green")
    
    table.add_row("CSV File", str(csv_file.name))
    table.add_row("Merge Strategy", merge_strategy.title())
    table.add_row("Total Games Processed", str(results.get('total_processed', 0)))
    table.add_row("New Games Added", str(results.get('new_games', 0)))
    table.add_row("Existing Games Updated", str(results.get('updated_games', 0)))
    table.add_row("Games Skipped", str(results.get('skipped_games', 0)))
    table.add_row("Errors Encountered", str(results.get('errors', 0)))
    
    console.print(table)
    
    # Show errors if any
    if results.get('error_details'):
        console.print("\n[bold red]Error Details:[/bold red]")
        for i, error in enumerate(results['error_details'], 1):
            console.print(f"  {i}. {error}")
    
    # Show recommendations
    console.print("\n[bold blue]Recommendations:[/bold blue]")
    if results.get('errors', 0) == 0:
        console.print("  ✓ Import completed without errors")
    else:
        console.print("  • Review error details above")
        console.print("  • Consider manual addition of failed games")
        console.print("  • Check platform mappings for unknown platforms")


if __name__ == "__main__":
    import_csv()