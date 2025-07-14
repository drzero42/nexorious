import { browser } from '$app/environment';
import { auth } from './auth.svelte.js';
import type { Game } from './games.svelte.js';

export interface WishlistItem {
  id: string;
  game: Game;
  created_at: string;
}

export interface WishlistState {
  wishlistItems: WishlistItem[];
  isLoading: boolean;
  error: string | null;
}

const initialState: WishlistState = {
  wishlistItems: [],
  isLoading: false,
  error: null
};

function createWishlistStore() {
  let state = $state<WishlistState>(initialState);

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const authState = auth.value;
    if (!authState.accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authState.accessToken}`,
        ...options.headers,
      },
    });

    if (!response.ok) {
      if (response.status === 401) {
        // Try to refresh token
        const refreshed = await auth.refreshAuth();
        if (refreshed) {
          // Retry the request with new token
          return fetch(url, {
            ...options,
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${auth.value.accessToken}`,
              ...options.headers,
            },
          });
        }
      }
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    return response;
  };

  return {
    get value() {
      return state;
    },

    // Load user's wishlist
    loadWishlist: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall('/api/wishlist');
        const wishlistItems: WishlistItem[] = await response.json();

        state = {
          ...state,
          wishlistItems,
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load wishlist';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Add game to wishlist
    addToWishlist: async (gameId: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall('/api/wishlist', {
          method: 'POST',
          body: JSON.stringify({ game_id: gameId }),
        });
        
        const wishlistItem: WishlistItem = await response.json();

        state = {
          ...state,
          wishlistItems: [wishlistItem, ...state.wishlistItems],
          isLoading: false
        };

        return wishlistItem;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to add game to wishlist';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Remove game from wishlist
    removeFromWishlist: async (gameId: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`/api/wishlist/${gameId}`, {
          method: 'DELETE',
        });

        state = {
          ...state,
          wishlistItems: state.wishlistItems.filter(item => item.game.id !== gameId),
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove game from wishlist';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Check if game is in wishlist
    isInWishlist: (gameId: string) => {
      return state.wishlistItems.some(item => item.game.id === gameId);
    },

    // Get wishlist item by game ID
    getWishlistItem: (gameId: string) => {
      return state.wishlistItems.find(item => item.game.id === gameId);
    },

    // Toggle game in wishlist
    toggleWishlist: async (gameId: string) => {
      if (this.isInWishlist(gameId)) {
        await this.removeFromWishlist(gameId);
      } else {
        await this.addToWishlist(gameId);
      }
    },

    // Move game from wishlist to collection
    moveToCollection: async (gameId: string) => {
      try {
        // First, add to collection
        const { userGames } = await import('./user-games.svelte.js');
        await userGames.addGameToCollection({ game_id: gameId });

        // Then remove from wishlist
        await this.removeFromWishlist(gameId);
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to move game to collection';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Get wishlist games by genre
    getGamesByGenre: (genre: string) => {
      return state.wishlistItems.filter(item => 
        item.game.genre?.toLowerCase().includes(genre.toLowerCase())
      );
    },

    // Get wishlist games by developer
    getGamesByDeveloper: (developer: string) => {
      return state.wishlistItems.filter(item => 
        item.game.developer?.toLowerCase().includes(developer.toLowerCase())
      );
    },

    // Get wishlist games by release year
    getGamesByYear: (year: number) => {
      return state.wishlistItems.filter(item => {
        if (!item.game.release_date) return false;
        return new Date(item.game.release_date).getFullYear() === year;
      });
    },

    // Search wishlist
    searchWishlist: (query: string) => {
      if (!query.trim()) return state.wishlistItems;
      
      const lowerQuery = query.toLowerCase();
      return state.wishlistItems.filter(item => 
        item.game.title.toLowerCase().includes(lowerQuery) ||
        item.game.description?.toLowerCase().includes(lowerQuery) ||
        item.game.genre?.toLowerCase().includes(lowerQuery) ||
        item.game.developer?.toLowerCase().includes(lowerQuery)
      );
    },

    // Sort wishlist
    sortWishlist: (sortBy: 'title' | 'release_date' | 'created_at', order: 'asc' | 'desc' = 'asc') => {
      const sorted = [...state.wishlistItems].sort((a, b) => {
        let aValue: any, bValue: any;

        switch (sortBy) {
          case 'title':
            aValue = a.game.title.toLowerCase();
            bValue = b.game.title.toLowerCase();
            break;
          case 'release_date':
            aValue = a.game.release_date ? new Date(a.game.release_date) : new Date(0);
            bValue = b.game.release_date ? new Date(b.game.release_date) : new Date(0);
            break;
          case 'created_at':
            aValue = new Date(a.created_at);
            bValue = new Date(b.created_at);
            break;
          default:
            return 0;
        }

        if (aValue < bValue) return order === 'asc' ? -1 : 1;
        if (aValue > bValue) return order === 'asc' ? 1 : -1;
        return 0;
      });

      state = { ...state, wishlistItems: sorted };
    },

    // Generate price comparison links
    generatePriceLinks: (game: Game) => {
      const links: Record<string, string> = {};
      
      // IsThereAnyDeal.com link
      if (game.title) {
        const searchTitle = encodeURIComponent(game.title);
        links.itad = `https://isthereanydeal.com/search/?q=${searchTitle}`;
      }
      
      // PSPrices.com link (for PlayStation games)
      if (game.title) {
        const searchTitle = encodeURIComponent(game.title);
        links.psprices = `https://psprices.com/region-us/search/?q=${searchTitle}`;
      }
      
      // Steam link (if available)
      if (game.metadata) {
        try {
          const metadata = JSON.parse(game.game_metadata);
          if (metadata.steam_id) {
            links.steam = `https://store.steampowered.com/app/${metadata.steam_id}/`;
          }
        } catch (error) {
          // Ignore parsing errors
        }
      }
      
      return links;
    },

    // Get wishlist statistics
    getWishlistStats: () => {
      const total = state.wishlistItems.length;
      const byGenre: Record<string, number> = {};
      const byDeveloper: Record<string, number> = {};
      const byYear: Record<string, number> = {};
      
      state.wishlistItems.forEach(item => {
        // Count by genre
        if (item.game.genre) {
          byGenre[item.game.genre] = (byGenre[item.game.genre] || 0) + 1;
        }
        
        // Count by developer
        if (item.game.developer) {
          byDeveloper[item.game.developer] = (byDeveloper[item.game.developer] || 0) + 1;
        }
        
        // Count by year
        if (item.game.release_date) {
          const year = new Date(item.game.release_date).getFullYear().toString();
          byYear[year] = (byYear[year] || 0) + 1;
        }
      });

      return {
        total,
        byGenre,
        byDeveloper,
        byYear,
        oldestGame: state.wishlistItems
          .filter(item => item.game.release_date)
          .sort((a, b) => new Date(a.game.release_date!).getTime() - new Date(b.game.release_date!).getTime())[0],
        newestGame: state.wishlistItems
          .filter(item => item.game.release_date)
          .sort((a, b) => new Date(b.game.release_date!).getTime() - new Date(a.game.release_date!).getTime())[0]
      };
    },

    // Clear wishlist
    clearWishlist: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        // Remove all items one by one (if no bulk delete endpoint)
        const promises = state.wishlistItems.map(item => 
          this.removeFromWishlist(item.game.id)
        );
        
        await Promise.all(promises);
        
        state = {
          ...state,
          wishlistItems: [],
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to clear wishlist';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Clear error
    clearError: () => {
      state = { ...state, error: null };
    }
  };
}

export const wishlist = createWishlistStore();