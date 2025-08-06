#!/usr/bin/env python3
"""
Darkadia CSV Import Script for Nexorious

This script imports game collections from Darkadia CSV export files into
the Nexorious game collection management system. Games are imported for
the authenticated user only.

Usage:
    python import_darkadia_csv.py [CSV_FILE] [OPTIONS]

Authentication:
    Must provide either --auth-token OR --username/--password
    Games will be imported for the authenticated user

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

# Add the parent directory to the path so we can import app modules
sys.path.insert(0, str(Path(__file__).parent.parent))

from scripts.darkadia.parser import DarkadiaCSVParser
from scripts.darkadia.api_client import NexoriousAPIClient
from scripts.darkadia.merge_strategies import MergeStrategy, InteractiveMerger, OverwriteMerger, PreserveMerger

console = Console()


@click.command()
@click.argument('csv_file', type=click.Path(exists=True, dir_okay=False, path_type=Path))
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
def import_csv(
    csv_file: Path,
    api_base: str,
    merge_strategy: str,
    dry_run: bool,
    batch_size: int,
    auth_token: Optional[str],
    username: Optional[str],
    password: Optional[str],
    verbose: bool
):
    """Import games from Darkadia CSV export file for the authenticated user."""
    
    console.print("[bold green]Darkadia CSV Import Tool[/bold green]")
    console.print(f"CSV File: {csv_file}")
    console.print(f"Merge Strategy: {merge_strategy}")
    
    if dry_run:
        console.print("[yellow]DRY RUN MODE - No changes will be made[/yellow]")
    
    try:
        # Run the async import process
        asyncio.run(run_import(
            csv_file=csv_file,
            api_base=api_base,
            merge_strategy=merge_strategy,
            dry_run=dry_run,
            batch_size=batch_size,
            auth_token=auth_token,
            username=username,
            password=password,
            verbose=verbose
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
    api_base: str,
    merge_strategy: str,
    dry_run: bool,
    batch_size: int,
    auth_token: Optional[str],
    username: Optional[str],
    password: Optional[str],
    verbose: bool
):
    """Run the main import process for the authenticated user."""
    
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
    
    # Phase 4: Get current user information
    console.print("\n[cyan]Phase 4: Retrieving user information...[/cyan]")
    current_user = await api_client.get_current_user()
    user_id = current_user.get('id')
    username_display = current_user.get('username')
    
    if not user_id:
        console.print("[red]Error: Unable to retrieve user ID from profile[/red]")
        return
    
    console.print(f"✓ Target User: {username_display}")
    
    # Phase 5: Setup merge strategy
    console.print(f"\n[cyan]Phase 5: Setting up {merge_strategy} merge strategy...[/cyan]")
    
    if merge_strategy == 'interactive':
        merger = InteractiveMerger(console, api_client, dry_run)
    elif merge_strategy == 'overwrite':
        merger = OverwriteMerger(api_client, dry_run)
    elif merge_strategy == 'preserve':
        merger = PreserveMerger(api_client, dry_run)
    else:
        raise ValueError(f"Unknown merge strategy: {merge_strategy}")
    
    console.print("✓ Merge strategy configured")
    
    # Phase 6: Process games
    console.print(f"\n[cyan]Phase 6: Processing {len(unique_games)} games...[/cyan]")
    
    results = await merger.process_games(
        unique_games, 
        user_id, 
        batch_size=batch_size
    )
    
    # Phase 7: Generate report
    console.print("\n[cyan]Phase 7: Generating report...[/cyan]")
    generate_final_report(results, csv_file, merge_strategy)
    
    console.print("\n[bold green]Import completed successfully![/bold green]")


def generate_final_report(results: dict, csv_file: Path, merge_strategy: str):
    """Generate and display the comprehensive final import report."""
    
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
    
    # Show detailed error analysis if any errors occurred
    structured_errors = results.get('structured_errors', [])
    if structured_errors:
        _show_categorized_errors(structured_errors)
        _show_detailed_error_list(structured_errors)
        _show_troubleshooting_guidance(structured_errors)
    elif results.get('error_details'):
        # Fallback for legacy error format
        console.print("\n[bold red]Error Details:[/bold red]")
        for i, error in enumerate(results['error_details'], 1):
            console.print(f"  {i}. {error}")
    
    # Show success/recommendations
    _show_final_recommendations(results, structured_errors)


def _show_categorized_errors(structured_errors):
    """Show errors grouped by category."""
    console.print("\n[bold red]Error Summary by Category[/bold red]")
    
    # Group errors by category
    error_categories = {}
    for error in structured_errors:
        category = error.category.value
        if category not in error_categories:
            error_categories[category] = []
        error_categories[category].append(error)
    
    # Create category summary table
    category_table = Table(title="Error Categories")
    category_table.add_column("Category", style="red")
    category_table.add_column("Count", justify="right", style="bold red")
    category_table.add_column("Description", style="dim")
    
    category_descriptions = {
        "csv_data": "Issues with CSV data format or content",
        "game_creation": "Failed to create new games",
        "game_update": "Failed to update existing games",
        "platform_mapping": "Platform or storefront mapping issues",
        "api_validation": "API validation errors",
        "authentication": "Authentication or authorization issues",
        "network": "Network connectivity problems",
        "igdb_integration": "IGDB API integration issues",
        "unexpected": "Unexpected or unknown errors"
    }
    
    for category, errors in sorted(error_categories.items()):
        description = category_descriptions.get(category, "Unknown error category")
        category_table.add_row(
            category.replace('_', ' ').title(),
            str(len(errors)),
            description
        )
    
    console.print(category_table)


def _show_detailed_error_list(structured_errors):
    """Show detailed list of all errors with context."""
    console.print("\n[bold red]Detailed Error Information[/bold red]")
    
    for i, error in enumerate(structured_errors, 1):
        console.print(f"\n[bold red]{i}. {error.category.value.replace('_', ' ').title()} Error[/bold red]")
        
        # Show detailed message
        detailed_msg = error.get_detailed_message()
        console.print(f"   {detailed_msg}")
        
        # Show API error details if available
        if error.api_error:
            # Show validation errors
            if error.api_error.validation_errors:
                console.print("   [dim]Validation Details:[/dim]")
                for val_error in error.api_error.validation_errors:
                    field = val_error.get('field', 'unknown')
                    message = val_error.get('message', 'validation failed')
                    console.print(f"     • {field}: {message}")
            
            # Show conflict details for 409 errors
            if error.api_error.conflict_details:
                console.print("   [dim]Conflict Details:[/dim]")
                conflict = error.api_error.conflict_details
                
                conflict_type = conflict.get('type', 'unknown')
                reason = conflict.get('reason', 'Unknown conflict')
                recommendation = conflict.get('recommendation', 'Review the conflict')
                
                console.print(f"     • Reason: {reason}")
                
                if conflict_type == 'duplicate_title':
                    title = conflict.get('conflicting_title', 'Unknown')
                    console.print(f"     • Existing title: '{title}'")
                elif conflict_type == 'duplicate_igdb_id':
                    igdb_id = conflict.get('conflicting_igdb_id', 'Unknown')
                    console.print(f"     • Conflicting IGDB ID: {igdb_id}")
                
                console.print(f"     • Recommendation: {recommendation}")
        
        # Show CSV data context if available
        if error.csv_data and error.game_title:
            console.print(f"   [dim]CSV Context: Game '{error.game_title}'[/dim]")
            if error.csv_row:
                console.print(f"   [dim]CSV Row: {error.csv_row}[/dim]")
        
        # Show additional context
        if error.context:
            operation = error.context.get('operation')
            if operation:
                console.print(f"   [dim]Operation: {operation}[/dim]")


def _show_troubleshooting_guidance(structured_errors):
    """Show specific troubleshooting guidance based on error types."""
    console.print("\n[bold blue]Troubleshooting Guide[/bold blue]")
    
    # Group errors by category for targeted advice
    error_categories = {}
    for error in structured_errors:
        category = error.category.value
        if category not in error_categories:
            error_categories[category] = []
        error_categories[category].append(error)
    
    # Provide specific guidance for each category
    guidance_shown = set()
    
    for category, errors in error_categories.items():
        if category in guidance_shown:
            continue
        
        guidance_shown.add(category)
        
        console.print(f"\n[bold cyan]{category.replace('_', ' ').title()} Issues ({len(errors)} errors):[/bold cyan]")
        
        if category == "csv_data":
            console.print("  • Check CSV file format and ensure all required columns are present")
            console.print("  • Verify game titles are not empty or contain special characters")
            console.print("  • Review CSV encoding (should be UTF-8)")
            
        elif category == "game_creation":
            console.print("  • Verify API connectivity and authentication")
            console.print("  • Check if IGDB integration is working properly")
            console.print("  • Some games may not exist in IGDB database")
            console.print("  • Note: This system only supports IGDB-sourced games")
            console.print("  • Try searching for alternative game titles in IGDB")
            
        elif category == "game_update":
            console.print("  • Check if games still exist in your collection")  
            console.print("  • Verify you have permission to update these games")
            console.print("  • Review data validation errors for specific field issues")
            
        elif category == "platform_mapping":
            console.print("  • Check platform and storefront names in your CSV")
            console.print("  • Verify platform/storefront combinations are valid")
            console.print("  • Some platforms may need to be created by an administrator")
            
        elif category == "api_validation":
            console.print("  • Check data formats (dates, ratings, etc.)")
            console.print("  • Verify required fields are not missing")
            console.print("  • Review field length limits and constraints")
            
            # Check for conflicts specifically
            conflict_errors = [e for e in errors if e.api_error and e.api_error.conflict_details]
            if conflict_errors:
                console.print("  [yellow]Duplicate Game Conflicts:[/yellow]")
                for error in conflict_errors[:3]:  # Show up to 3 examples
                    conflict = error.api_error.conflict_details
                    if conflict.get('type') == 'duplicate_title':
                        console.print(f"    - '{error.game_title}': Title already exists - consider modifying the title")
                    elif conflict.get('type') == 'duplicate_igdb_id':
                        console.print(f"    - '{error.game_title}': Exact game already in database - consider skipping")
                if len(conflict_errors) > 3:
                    console.print(f"    - And {len(conflict_errors) - 3} more conflicts...")
            
        elif category == "authentication":
            console.print("  • Verify username and password are correct")
            console.print("  • Check if your user account is active")
            console.print("  • Try logging in through the web interface first")
            
        elif category == "network":
            console.print("  • Check internet connectivity")
            console.print("  • Verify API server is running and accessible")
            console.print("  • Try again later if server is temporarily unavailable")
            
        elif category == "igdb_integration":
            console.print("  • IGDB API may be temporarily unavailable")
            console.print("  • Some games may not exist in IGDB database")
            console.print("  • Check IGDB API rate limits")
            
        # Show specific examples from errors if available
        example_games = [e.game_title for e in errors[:3] if e.game_title]
        if example_games:
            console.print(f"  [dim]Affected games: {', '.join(example_games)}{' (and others)' if len(errors) > 3 else ''}[/dim]")


def _show_final_recommendations(results, structured_errors):
    """Show final recommendations and next steps."""
    console.print("\n[bold blue]Next Steps & Recommendations[/bold blue]")
    
    total_errors = results.get('errors', 0)
    total_processed = results.get('total_processed', 0)
    
    if total_errors == 0:
        console.print("  ✓ [green]Import completed successfully without errors![/green]")
        console.print("  ✓ All games have been processed and added to your collection")
    else:
        success_rate = ((total_processed - total_errors) / total_processed * 100) if total_processed > 0 else 0
        console.print(f"  • [yellow]Import completed with {total_errors} errors[/yellow]")
        console.print(f"  • [green]Success rate: {success_rate:.1f}% ({total_processed - total_errors}/{total_processed} games)[/green]")
        
        # Specific recommendations based on error types
        if structured_errors:
            critical_categories = ["authentication", "network", "csv_data"]
            critical_errors = [e for e in structured_errors if e.category.value in critical_categories]
            
            if critical_errors:
                console.print("  • [red]Critical issues detected - fix these first:[/red]")
                for category in critical_categories:
                    count = len([e for e in critical_errors if e.category.value == category])
                    if count > 0:
                        console.print(f"    - {category.replace('_', ' ').title()}: {count} errors")
            
            # Suggest retry for certain error types
            retry_categories = ["network", "igdb_integration"]
            retry_errors = [e for e in structured_errors if e.category.value in retry_categories]
            if retry_errors:
                console.print(f"  • [yellow]Consider retrying import - {len(retry_errors)} errors may be temporary[/yellow]")
        
        console.print("  • Review the detailed error information above")
        console.print("  • Fix issues in your CSV file and retry import")
        console.print("  • Contact support if problems persist")
    
    # Show import summary
    new_games = results.get('new_games', 0)
    updated_games = results.get('updated_games', 0)
    
    if new_games > 0:
        console.print(f"  ✓ [green]{new_games} new games added to your collection[/green]")
    if updated_games > 0:
        console.print(f"  ✓ [green]{updated_games} existing games updated[/green]")


if __name__ == "__main__":
    import_csv()