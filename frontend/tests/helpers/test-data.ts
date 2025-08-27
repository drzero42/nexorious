/**
 * Test data factories for consistent E2E test data generation
 */

export interface TestGameData {
  title: string;
  description?: string;
  personalRating?: string;
  playStatus?: 'not_started' | 'in_progress' | 'completed' | 'abandoned' | 'on_hold';
  ownershipStatus?: 'owned' | 'wishlist' | 'not_owned' | 'borrowed' | 'subscription';
  hoursPlayed?: string;
  platforms?: string[];
  personalNotes?: string;
}

export interface TestIGDBGame {
  id: number;
  name: string;
  summary?: string;
  first_release_date?: number;
  cover?: {
    url: string;
  };
  platforms?: Array<{
    id: number;
    name: string;
  }>;
  genres?: Array<{
    id: number;
    name: string;
  }>;
}

/**
 * Factory for creating test game data
 */
export class TestGameFactory {
  private static gameCounter = 1;

  static create(overrides: Partial<TestGameData> = {}): TestGameData {
    const id = this.gameCounter++;
    
    return {
      title: `Test Game ${id}`,
      description: `A test game for E2E testing purposes (${id})`,
      personalRating: '7',
      playStatus: 'not_started',
      ownershipStatus: 'owned',
      hoursPlayed: '0',
      platforms: ['PC'],
      personalNotes: `Test notes for game ${id}`,
      ...overrides
    };
  }

  /**
   * Create a game with specific characteristics for testing
   */
  static createInProgress(overrides: Partial<TestGameData> = {}): TestGameData {
    return this.create({
      playStatus: 'in_progress',
      hoursPlayed: '25',
      personalRating: '8',
      personalNotes: 'Currently playing this game',
      ...overrides
    });
  }

  static createCompleted(overrides: Partial<TestGameData> = {}): TestGameData {
    return this.create({
      playStatus: 'completed',
      hoursPlayed: '50',
      personalRating: '9',
      personalNotes: 'Great game, finished it!',
      ...overrides
    });
  }

  static createWishlistGame(overrides: Partial<TestGameData> = {}): TestGameData {
    return this.create({
      ownershipStatus: 'wishlist',
      playStatus: 'not_started',
      hoursPlayed: '0',
      personalNotes: 'Want to play this game',
      ...overrides
    });
  }

  /**
   * Create multiple test games at once
   */
  static createBatch(count: number, template: Partial<TestGameData> = {}): TestGameData[] {
    return Array.from({ length: count }, (_, index) => 
      this.create({ 
        title: `${template.title || 'Batch Game'} ${index + 1}`,
        ...template 
      })
    );
  }

  /**
   * Reset counter for predictable test data
   */
  static reset(): void {
    this.gameCounter = 1;
  }
}

/**
 * Mock IGDB API responses for predictable testing
 */
export class MockIGDBResponses {
  /**
   * Mock successful search results
   */
  static searchResults(query: string): TestIGDBGame[] {
    // Return different results based on query for testing
    if (query.toLowerCase().includes('witcher')) {
      return [
        {
          id: 1942,
          name: 'The Witcher 3: Wild Hunt',
          summary: 'The Witcher 3: Wild Hunt is a story-driven, next-generation open world role-playing game set in a visually stunning fantasy universe full of meaningful choices and impactful consequences.',
          first_release_date: 1431475200, // May 19, 2015
          cover: {
            url: 'https://images.igdb.com/igdb/image/upload/t_cover_big/co1wyy.webp'
          },
          platforms: [
            { id: 6, name: 'PC (Microsoft Windows)' },
            { id: 48, name: 'PlayStation 4' },
            { id: 49, name: 'Xbox One' }
          ],
          genres: [
            { id: 12, name: 'Role-playing (RPG)' },
            { id: 31, name: 'Adventure' }
          ]
        },
        {
          id: 1020,
          name: 'The Witcher',
          summary: 'The Witcher is a role-playing game set in a dark fantasy world.',
          first_release_date: 1161129600, // Oct 26, 2007
          cover: {
            url: 'https://images.igdb.com/igdb/image/upload/t_cover_big/co1ab2.webp'
          },
          platforms: [
            { id: 6, name: 'PC (Microsoft Windows)' }
          ],
          genres: [
            { id: 12, name: 'Role-playing (RPG)' }
          ]
        }
      ];
    }

    if (query.toLowerCase().includes('cyberpunk')) {
      return [
        {
          id: 1877,
          name: 'Cyberpunk 2077',
          summary: 'Cyberpunk 2077 is an open-world, action-adventure story set in Night City.',
          first_release_date: 1607558400, // Dec 10, 2020
          cover: {
            url: 'https://images.igdb.com/igdb/image/upload/t_cover_big/co2dpz.webp'
          },
          platforms: [
            { id: 6, name: 'PC (Microsoft Windows)' },
            { id: 48, name: 'PlayStation 4' },
            { id: 49, name: 'Xbox One' }
          ],
          genres: [
            { id: 12, name: 'Role-playing (RPG)' },
            { id: 5, name: 'Shooter' }
          ]
        }
      ];
    }

    // Return empty results for unknown queries
    return [];
  }

  /**
   * Mock API error response
   */
  static errorResponse(status: number = 500): { error: string; status: number } {
    return {
      error: 'IGDB API temporarily unavailable',
      status
    };
  }

  /**
   * Mock network timeout
   */
  static timeoutResponse(): Promise<never> {
    return new Promise((_, reject) => {
      setTimeout(() => {
        reject(new Error('Request timeout'));
      }, 30000);
    });
  }
}

/**
 * Common test scenarios for different game management workflows
 */
export class TestScenarios {
  /**
   * Standard game creation scenario
   */
  static readonly BASIC_GAME_CREATION = {
    searchQuery: 'The Witcher 3',
    expectedResults: MockIGDBResponses.searchResults('The Witcher 3'),
    gameData: TestGameFactory.create({
      title: 'The Witcher 3: Wild Hunt',
      personalRating: '9',
      playStatus: 'not_started',
      platforms: ['PC', 'PlayStation 4']
    })
  };

  /**
   * Manual game creation scenario (no IGDB results)
   */
  static readonly MANUAL_GAME_CREATION = {
    searchQuery: 'NonExistentGame12345',
    expectedResults: [],
    gameData: TestGameFactory.create({
      title: 'My Custom Game',
      description: 'A manually added game for testing',
      personalRating: '8',
      playStatus: 'in_progress',
      hoursPlayed: '10'
    })
  };

  /**
   * Game editing scenario
   */
  static readonly GAME_EDITING = {
    originalGame: TestGameFactory.create({
      title: 'Game to Edit',
      personalRating: '6',
      playStatus: 'not_started',
      hoursPlayed: '0'
    }),
    updates: {
      personalRating: '8',
      playStatus: 'in_progress',
      hoursPlayed: '15',
      personalNotes: 'Updated notes after playing'
    }
  };

  /**
   * Bulk operations scenario
   */
  static readonly BULK_OPERATIONS = {
    games: TestGameFactory.createBatch(5, { 
      personalRating: '7',
      playStatus: 'not_started' 
    }),
    bulkUpdate: {
      playStatus: 'in_progress',
      personalRating: '8'
    }
  };
}

/**
 * Common assertions for game-related E2E tests
 */
export class GameAssertions {
  /**
   * Assert that a game appears in the games list with expected data
   */
  static gameInList(gameTitle: string) {
    return {
      selector: `[data-testid="game-card"][data-game-title="${gameTitle}"]`,
      text: gameTitle
    };
  }

  /**
   * Assert that game details page shows correct information
   */
  static gameDetails(gameData: TestGameData) {
    return {
      title: gameData.title,
      rating: gameData.personalRating,
      status: gameData.playStatus,
      hours: gameData.hoursPlayed,
      notes: gameData.personalNotes
    };
  }

  /**
   * Assert that form validation messages are correct
   */
  static validationMessages = {
    REQUIRED_TITLE: /title is required/i,
    INVALID_RATING: /rating must be between/i,
    NEGATIVE_HOURS: /hours cannot be negative/i,
    MIN_SEARCH_LENGTH: /search term must be at least/i,
    EMPTY_SEARCH: /please enter a game name/i,
    PASSWORD_MISMATCH: /passwords do not match/i
  };

  /**
   * Assert that loading states are displayed correctly
   */
  static loadingStates = {
    SEARCHING: /searching/i,
    SAVING: /saving/i,
    LOADING: /loading/i,
    UPDATING: /updating/i,
    DELETING: /deleting/i
  };

  /**
   * Assert that success messages are shown
   */
  static successMessages = {
    GAME_ADDED: /game added successfully/i,
    GAME_UPDATED: /updated successfully/i,
    GAME_DELETED: /deleted successfully/i,
    SETTINGS_SAVED: /settings saved/i
  };

  /**
   * Assert that error messages are appropriate
   */
  static errorMessages = {
    NETWORK_ERROR: /network error|connection failed/i,
    API_ERROR: /server error|api unavailable/i,
    NOT_FOUND: /not found/i,
    UNAUTHORIZED: /unauthorized|access denied/i,
    VALIDATION_ERROR: /invalid input|validation failed/i
  };
}