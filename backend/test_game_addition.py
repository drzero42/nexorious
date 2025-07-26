#!/usr/bin/env python3
"""Test script to verify game addition to user collection."""

import requests
import json
from datetime import datetime

# API base URL
BASE_URL = "http://localhost:8000/api"

# Test user credentials
TEST_USER = {
    "username": "testuser_debug",
    "password": "TestPassword123!"
}

def register_or_login():
    """Register a test user or login if already exists."""
    # Try to register first
    try:
        response = requests.post(f"{BASE_URL}/auth/register", json=TEST_USER)
        if response.status_code == 201:
            print("✓ User registered successfully")
            data = response.json()
            return data["access_token"]
    except:
        pass
    
    # If registration fails, try to login
    login_data = {
        "username": TEST_USER["username"],
        "password": TEST_USER["password"]
    }
    response = requests.post(f"{BASE_URL}/auth/login", json=login_data)
    if response.status_code == 200:
        print("✓ User logged in successfully")
        data = response.json()
        return data["access_token"]
    else:
        print(f"✗ Login failed: {response.status_code} - {response.text}")
        return None

def create_test_game(token):
    """Create a test game."""
    headers = {"Authorization": f"Bearer {token}"}
    
    game_data = {
        "title": f"Test Game {datetime.now().strftime('%Y%m%d%H%M%S')}",
        "description": "A test game for debugging",
        "genre": "Action",
        "developer": "Test Developer",
        "publisher": "Test Publisher",
        "release_date": "2024-01-01"
    }
    
    response = requests.post(f"{BASE_URL}/games", json=game_data, headers=headers)
    if response.status_code == 201:
        game = response.json()
        print(f"✓ Game created: {game['title']} (ID: {game['id']})")
        return game["id"]
    else:
        print(f"✗ Game creation failed: {response.status_code} - {response.text}")
        return None

def get_platforms(token):
    """Get available platforms."""
    headers = {"Authorization": f"Bearer {token}"}
    
    response = requests.get(f"{BASE_URL}/platforms", headers=headers)
    if response.status_code == 200:
        platforms = response.json()
        if platforms:
            print(f"✓ Found {len(platforms)} platforms")
            return [p["id"] for p in platforms[:2]]  # Return first 2 platform IDs
        else:
            print("✗ No platforms found")
            return []
    else:
        print(f"✗ Failed to get platforms: {response.status_code} - {response.text}")
        return []

def add_game_to_collection(token, game_id, platform_ids):
    """Add a game to user's collection."""
    headers = {"Authorization": f"Bearer {token}"}
    
    collection_data = {
        "game_id": game_id,
        "ownership_status": "owned",
        "is_physical": False,
        "play_status": "not_started",
        "platforms": platform_ids
    }
    
    print(f"\nAdding game to collection with data: {json.dumps(collection_data, indent=2)}")
    
    response = requests.post(f"{BASE_URL}/user-games", json=collection_data, headers=headers)
    if response.status_code == 201:
        user_game = response.json()
        print(f"✓ Game added to collection (UserGame ID: {user_game['id']})")
        return user_game["id"]
    else:
        print(f"✗ Failed to add game to collection: {response.status_code} - {response.text}")
        return None

def verify_user_games(token):
    """Verify user's game collection."""
    headers = {"Authorization": f"Bearer {token}"}
    
    response = requests.get(f"{BASE_URL}/user-games", headers=headers)
    if response.status_code == 200:
        data = response.json()
        print(f"\n✓ User has {data['total']} games in collection")
        for game in data['user_games']:
            print(f"  - {game['game']['title']} (Status: {game['play_status']}, Platforms: {len(game['platforms'])})")
    else:
        print(f"✗ Failed to get user games: {response.status_code} - {response.text}")

def main():
    """Run the test."""
    print("=== Testing Game Addition Flow ===\n")
    
    # Step 1: Login
    token = register_or_login()
    if not token:
        print("✗ Authentication failed")
        return
    
    # Step 2: Create a test game
    game_id = create_test_game(token)
    if not game_id:
        print("✗ Game creation failed")
        return
    
    # Step 3: Get available platforms
    platform_ids = get_platforms(token)
    
    # Step 4: Add game to collection
    user_game_id = add_game_to_collection(token, game_id, platform_ids)
    
    # Step 5: Verify collection
    verify_user_games(token)
    
    print("\n=== Test Complete ===")

if __name__ == "__main__":
    main()