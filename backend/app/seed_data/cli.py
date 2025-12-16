"""
CLI tool for seeding platforms and storefronts.
"""

import argparse
import sys
from sqlmodel import Session

from ..core.database import get_engine
from .seeder import seed_all_official_data, get_seeding_conflicts


def main():
    """Main CLI entry point."""
    parser = argparse.ArgumentParser(description="Seed platforms and storefronts")
    parser.add_argument(
        "--version", 
        default="1.0.0", 
        help="Version string for tracking when data was added"
    )
    parser.add_argument(
        "--check-conflicts", 
        action="store_true", 
        help="Check for potential conflicts without seeding"
    )
    parser.add_argument(
        "--force", 
        action="store_true", 
        help="Force seeding even with conflicts"
    )
    
    args = parser.parse_args()
    
    # Use existing database engine and session
    with Session(get_engine()) as session:
        if args.check_conflicts:
            conflicts = get_seeding_conflicts(session)
            if conflicts["platforms"] or conflicts["storefronts"]:
                print("⚠️  Potential conflicts found:")
                if conflicts["platforms"]:
                    print(f"  Platforms: {', '.join(conflicts['platforms'])}")
                if conflicts["storefronts"]:
                    print(f"  Storefronts: {', '.join(conflicts['storefronts'])}")
                print("\nThese custom entries will be converted to official ones during seeding.")
            else:
                print("✅ No conflicts found. Safe to seed.")
            return
        
        # Check for conflicts unless forced
        if not args.force:
            conflicts = get_seeding_conflicts(session)
            if conflicts["platforms"] or conflicts["storefronts"]:
                print("⚠️  Potential conflicts found:")
                if conflicts["platforms"]:
                    print(f"  Platforms: {', '.join(conflicts['platforms'])}")
                if conflicts["storefronts"]:
                    print(f"  Storefronts: {', '.join(conflicts['storefronts'])}")
                
                response = input("\nProceed with seeding? Custom entries will be converted to official (y/N): ")
                if response.lower() not in ['y', 'yes']:
                    print("Seeding cancelled.")
                    sys.exit(0)
        
        # Perform seeding
        print(f"🌱 Seeding official data for version {args.version}...")
        result = seed_all_official_data(session, args.version)
        
        print("✅ Seeding completed:")
        print(f"  Platforms: {result['platforms']}")
        print(f"  Storefronts: {result['storefronts']}")
        print(f"  Total: {result['total']}")


if __name__ == "__main__":
    main()