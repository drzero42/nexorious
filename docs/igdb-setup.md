# IGDB API Setup Guide

This guide explains how to obtain and configure IGDB API credentials for Nexorious. IGDB (Internet Game Database) provides the game metadata, cover art, and search functionality used throughout the application.

## Overview

Nexorious uses the IGDB API (via Twitch) for:

- Game search and discovery
- Importing game metadata (title, description, release date, etc.)
- Cover art downloads
- Time-to-beat information
- Platform data

Without IGDB credentials, these features will be unavailable.

## Prerequisites

- A Twitch account (free)
- Access to the Twitch Developer Console

## Step-by-Step Setup

### 1. Create a Twitch Account (if needed)

If you don't have a Twitch account:

1. Go to [twitch.tv](https://twitch.tv)
2. Click "Sign Up"
3. Complete the registration process

### 2. Enable Two-Factor Authentication

Twitch requires 2FA to access the Developer Console:

1. Go to [Twitch Security Settings](https://www.twitch.tv/settings/security)
2. Enable Two-Factor Authentication
3. Follow the prompts to set up 2FA

### 3. Register a Twitch Application

1. Go to the [Twitch Developer Console](https://dev.twitch.tv/console)
2. Log in with your Twitch account
3. Click "Register Your Application"
4. Fill in the application details:
   - **Name**: Choose any name (e.g., "Nexorious")
   - **OAuth Redirect URLs**: `http://localhost` (this URL won't actually be used)
   - **Category**: Select "Application Integration"
5. Click "Create"

### 4. Get Your Credentials

After creating the application:

1. Click "Manage" on your newly created application
2. Note down your **Client ID**
3. Click "New Secret" to generate a **Client Secret**
4. Copy and save the Client Secret immediately (it won't be shown again)

### 5. Configure Nexorious

Add the credentials to your environment configuration:

#### Option A: Using a `.env` file (recommended for development)

Create or edit the `.env` file in the `backend` directory:

```bash
IGDB_CLIENT_ID=your_client_id_here
IGDB_CLIENT_SECRET=your_client_secret_here
```

#### Option B: Using Docker Compose

Add the environment variables to your `docker-compose.yml`:

```yaml
services:
  api:
    environment:
      - IGDB_CLIENT_ID=your_client_id_here
      - IGDB_CLIENT_SECRET=your_client_secret_here
```

#### Option C: Using system environment variables

```bash
export IGDB_CLIENT_ID=your_client_id_here
export IGDB_CLIENT_SECRET=your_client_secret_here
```

### 6. Restart the Backend

After configuring the credentials, restart the Nexorious backend:

```bash
# If using Docker Compose
docker-compose restart api

# If running directly
# Stop the current process and restart
cd backend
uv run python -m app.main
```

## Verification

To verify your IGDB integration is working:

1. Check the backend logs on startup - you should NOT see a warning about missing IGDB credentials
2. Navigate to the "Add Game" page and try searching for a game
3. Check the `/api/status` endpoint - it should return `{"igdb_configured": true}`

## Troubleshooting

### "IGDB credentials not configured" warning in logs

**Cause**: The `IGDB_CLIENT_ID` or `IGDB_CLIENT_SECRET` environment variables are not set or are empty.

**Solution**: Double-check your environment configuration and ensure both variables are set correctly.

### "IGDB authentication failed" error

**Possible causes**:
- Invalid Client ID or Client Secret
- Client Secret may have been rotated/regenerated

**Solution**:
1. Go to the [Twitch Developer Console](https://dev.twitch.tv/console)
2. Verify your Client ID matches
3. Generate a new Client Secret if needed
4. Update your configuration and restart

### Game search returns no results

**Possible causes**:
- IGDB may not have the game in their database
- Search query may be too specific

**Solution**:
- Try simplifying the search query
- Check if the game exists on [IGDB.com](https://www.igdb.com)

### Rate limiting errors

**Cause**: Too many requests to the IGDB API in a short period.

**Solution**: Nexorious includes built-in rate limiting (4 requests/second by default). If you're still hitting limits, wait a few minutes before retrying.

## Security Notes

- Never commit your Client Secret to version control
- Use environment variables or secrets management in production
- The Client Secret can be rotated in the Twitch Developer Console if compromised

## Additional Resources

- [IGDB API Documentation](https://api-docs.igdb.com/)
- [Twitch Developer Portal](https://dev.twitch.tv/)
- [Twitch Authentication Guide](https://dev.twitch.tv/docs/authentication)
