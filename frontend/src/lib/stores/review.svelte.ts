/**
 * Review store for managing the review queue.
 *
 * Provides state management and API methods for listing review items,
 * viewing details, and resolving items (match, skip, keep, remove).
 */

import { auth } from './auth.svelte';
import { config } from '$lib/env';
import type {
  ReviewItem,
  ReviewItemDetail,
  ReviewListResponse,
  ReviewSummary,
  ReviewCountsByType,
  MatchResponse,
  ReviewFilters
} from '$lib/types/jobs';
import { ReviewItemStatus, ReviewSource } from '$lib/types/jobs';

// Re-export types and enums for convenience
export type { ReviewItem, ReviewItemDetail, ReviewSummary, ReviewCountsByType, ReviewFilters };
export { ReviewItemStatus, ReviewSource };

export interface ReviewState {
  items: ReviewItem[];
  currentItem: ReviewItemDetail | null;
  summary: ReviewSummary | null;
  countsByType: ReviewCountsByType | null;
  isLoading: boolean;
  error: string | null;
  filters: ReviewFilters;
  pagination: {
    page: number;
    per_page: number;
    total: number;
    pages: number;
  };
}

const initialState: ReviewState = {
  items: [],
  currentItem: null,
  summary: null,
  countsByType: null,
  isLoading: false,
  error: null,
  filters: {},
  pagination: {
    page: 1,
    per_page: 20,
    total: 0,
    pages: 0
  }
};

function createReviewStore() {
  let state = $state<ReviewState>({ ...initialState });

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const authState = auth.value;
    if (!authState.accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authState.accessToken}`,
        ...options.headers
      }
    });

    if (!response.ok) {
      if (response.status === 401) {
        const refreshed = await auth.refreshAuth();
        if (refreshed) {
          return fetch(url, {
            ...options,
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${auth.value.accessToken}`,
              ...options.headers
            }
          });
        }
      }

      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      try {
        const errorBody = await response.json();
        if (errorBody.detail) {
          errorMessage = errorBody.detail;
        }
      } catch {
        // Use default message if we can't parse the error body
      }

      throw new Error(errorMessage);
    }

    return response;
  };

  const store = {
    get value() {
      return state;
    },

    /**
     * Derived getter for pending item count.
     */
    get pendingCount() {
      return state.summary?.total_pending ?? 0;
    },

    /**
     * Load review items with optional filters and pagination.
     */
    loadItems: async (
      filters: ReviewFilters = {},
      page: number = 1,
      per_page: number = 20
    ) => {
      state.isLoading = true;
      state.error = null;

      try {
        const params = new URLSearchParams();

        if (filters.status) params.append('status', filters.status);
        if (filters.job_id) params.append('job_id', filters.job_id);
        if (filters.source) params.append('source', filters.source);

        params.append('page', page.toString());
        params.append('per_page', per_page.toString());

        const response = await apiCall(`${config.apiUrl}/review/?${params}`);
        const data: ReviewListResponse = await response.json();

        state.items = data.items;
        state.filters = filters;
        state.pagination = {
          page: data.page,
          per_page: data.per_page,
          total: data.total,
          pages: data.pages
        };
        state.isLoading = false;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load review items';
        state.isLoading = false;
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Get review summary statistics.
     */
    loadSummary: async () => {
      try {
        const response = await apiCall(`${config.apiUrl}/review/summary`);
        const summary: ReviewSummary = await response.json();

        state.summary = summary;
        return summary;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load review summary';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Get pending review counts grouped by job type (import vs sync).
     * Used by navigation badges to show how many items need review.
     */
    loadCountsByType: async () => {
      try {
        const response = await apiCall(`${config.apiUrl}/review/counts`);
        const counts: ReviewCountsByType = await response.json();

        state.countsByType = counts;
        return counts;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load review counts';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Get a specific review item by ID with full details.
     */
    getItem: async (itemId: string) => {
      state.isLoading = true;
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/review/${itemId}`);
        const item: ReviewItemDetail = await response.json();

        state.currentItem = item;
        state.isLoading = false;
        return item;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load review item';
        state.isLoading = false;
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Match a review item to an IGDB ID.
     */
    matchItem: async (itemId: string, igdbId: number): Promise<MatchResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/review/${itemId}/match`, {
          method: 'POST',
          body: JSON.stringify({ igdb_id: igdbId })
        });
        const result: MatchResponse = await response.json();

        if (result.success && result.item) {
          // Update in the list
          const index = state.items.findIndex((i) => i.id === itemId);
          if (index !== -1) {
            state.items[index] = result.item;
          }

          // Update current item if it's the same (preserve igdb_candidates type)
          if (state.currentItem?.id === itemId) {
            const { igdb_candidates: _, ...updatedFields } = result.item;
            state.currentItem = {
              ...state.currentItem,
              ...updatedFields
            };
          }

          // Update summary counts
          if (state.summary) {
            state.summary.total_pending = Math.max(0, state.summary.total_pending - 1);
            state.summary.total_matched++;
          }
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to match item';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Skip a review item without matching.
     */
    skipItem: async (itemId: string): Promise<MatchResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/review/${itemId}/skip`, {
          method: 'POST'
        });
        const result: MatchResponse = await response.json();

        if (result.success && result.item) {
          // Update in the list
          const index = state.items.findIndex((i) => i.id === itemId);
          if (index !== -1) {
            state.items[index] = result.item;
          }

          // Update current item if it's the same (preserve igdb_candidates type)
          if (state.currentItem?.id === itemId) {
            const { igdb_candidates: _, ...updatedFields } = result.item;
            state.currentItem = {
              ...state.currentItem,
              ...updatedFields
            };
          }

          // Update summary counts
          if (state.summary) {
            state.summary.total_pending = Math.max(0, state.summary.total_pending - 1);
            state.summary.total_skipped++;
          }
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to skip item';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Keep a game flagged for removal in the collection.
     */
    keepItem: async (itemId: string): Promise<MatchResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/review/${itemId}/keep`, {
          method: 'POST'
        });
        const result: MatchResponse = await response.json();

        if (result.success && result.item) {
          // Update in the list
          const index = state.items.findIndex((i) => i.id === itemId);
          if (index !== -1) {
            state.items[index] = result.item;
          }

          // Update current item if it's the same (preserve igdb_candidates type)
          if (state.currentItem?.id === itemId) {
            const { igdb_candidates: _, ...updatedFields } = result.item;
            state.currentItem = {
              ...state.currentItem,
              ...updatedFields
            };
          }

          // Update summary counts
          if (state.summary) {
            state.summary.total_pending = Math.max(0, state.summary.total_pending - 1);
            state.summary.total_matched++;
          }
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to keep item';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Remove a game flagged for removal from the collection.
     */
    removeItem: async (itemId: string): Promise<MatchResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/review/${itemId}/remove`, {
          method: 'POST'
        });
        const result: MatchResponse = await response.json();

        if (result.success && result.item) {
          // Update in the list
          const index = state.items.findIndex((i) => i.id === itemId);
          if (index !== -1) {
            state.items[index] = result.item;
          }

          // Update current item if it's the same (preserve igdb_candidates type)
          if (state.currentItem?.id === itemId) {
            const { igdb_candidates: _, ...updatedFields } = result.item;
            state.currentItem = {
              ...state.currentItem,
              ...updatedFields
            };
          }

          // Update summary counts
          if (state.summary) {
            state.summary.total_pending = Math.max(0, state.summary.total_pending - 1);
            state.summary.total_removal++;
          }
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove item';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Navigate to the next pending item in the queue.
     */
    nextPendingItem: async () => {
      // First, try to find the next pending item in the current list
      const currentIndex = state.currentItem
        ? state.items.findIndex((i) => i.id === state.currentItem?.id)
        : -1;

      // Find next pending item after current
      for (let i = currentIndex + 1; i < state.items.length; i++) {
        const item = state.items[i];
        if (item && item.status === ReviewItemStatus.PENDING) {
          return store.getItem(item.id);
        }
      }

      // If not found, check from the beginning
      for (let i = 0; i < currentIndex; i++) {
        const item = state.items[i];
        if (item && item.status === ReviewItemStatus.PENDING) {
          return store.getItem(item.id);
        }
      }

      // If no pending items in current list, load fresh pending items
      await store.loadItems({ status: ReviewItemStatus.PENDING }, 1, 20);

      const firstItem = state.items[0];
      if (firstItem && firstItem.status === ReviewItemStatus.PENDING) {
        return store.getItem(firstItem.id);
      }

      // No more pending items
      state.currentItem = null;
      return null;
    },

    /**
     * Refresh the items list with current filters.
     */
    refresh: async () => {
      return store.loadItems(state.filters, state.pagination.page, state.pagination.per_page);
    },

    /**
     * Clear the current item selection.
     */
    clearCurrentItem: () => {
      state.currentItem = null;
    },

    /**
     * Clear filters.
     */
    clearFilters: () => {
      state.filters = {};
    },

    /**
     * Clear error state.
     */
    clearError: () => {
      state.error = null;
    },

    /**
     * Reset store to initial state.
     */
    reset: () => {
      state = { ...initialState };
    }
  };

  return store;
}

export const review = createReviewStore();
