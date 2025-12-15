import { config } from '$lib/env';
import { loggers } from '$lib/services/logger';
import { api } from '$lib/services/api';

const log = loggers.tags;

// Tag Interface matching backend schema
export interface Tag {
  id: string;
  user_id: string;
  name: string;
  color: string; // Hex color code (e.g., "#6B7280")
  description?: string;
  created_at: string;
  updated_at: string;
  // Computed/frontend-only properties
  game_count?: number; // Number of games with this tag
}

// Request interfaces for API calls
export interface TagCreateRequest {
  name: string;
  color?: string; // Will default to "#6B7280" on backend
  description?: string;
}

export interface TagUpdateRequest {
  name?: string;
  color?: string;
  description?: string;
}

// Bulk operations
export interface BulkTagAssignRequest {
  user_game_ids: string[];
  tag_ids: string[];
}

export interface BulkTagRemoveRequest {
  user_game_ids: string[];
  tag_ids: string[];
}

// State management interfaces
export interface TagsState {
  tags: Tag[];
  currentTag: Tag | null;
  isLoading: boolean;
  error: string | null;
  // Tag usage statistics
  tagUsageMap: Map<string, number>; // tag_id -> game count
}

// Event system for cross-component synchronization
class TagEventBus {
  private listeners = new Map<string, Set<Function>>();

  emit(event: string, data: any) {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      callbacks.forEach(callback => callback(data));
    }
  }

  on(event: string, callback: Function) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(callback);
  }

  off(event: string, callback: Function) {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      callbacks.delete(callback);
    }
  }
}

export const tagEventBus = new TagEventBus();

// Color validation helper
const isValidHexColor = (color: string): boolean => {
  return /^#[0-9A-F]{6}$/i.test(color);
};

// Default tag colors for suggestions
export const DEFAULT_TAG_COLORS = [
  '#EF4444', // Red
  '#F97316', // Orange
  '#F59E0B', // Amber
  '#EAB308', // Yellow
  '#84CC16', // Lime
  '#22C55E', // Green
  '#10B981', // Emerald
  '#14B8A6', // Teal
  '#06B6D4', // Cyan
  '#0EA5E9', // Sky
  '#3B82F6', // Blue
  '#6366F1', // Indigo
  '#8B5CF6', // Violet
  '#A855F7', // Purple
  '#D946EF', // Fuchsia
  '#EC4899', // Pink
  '#F43F5E', // Rose
  '#6B7280', // Gray (default)
];

const initialState: TagsState = {
  tags: [],
  currentTag: null,
  isLoading: false,
  error: null,
  tagUsageMap: new Map()
};

function createTagsStore() {
  let state = $state<TagsState>(initialState);

  return {
    get value() {
      return state;
    },
    
    // Subscribe method for compatibility with Svelte store interface
    subscribe: (run: (value: TagsState) => void) => {
      run(state); // Initial call
      return () => {}; // Return unsubscribe function
    },

    // Fetch all tags for the current user
    fetchTags: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.get(`${config.apiUrl}/tags/`);
        const data = await response.json();

        state = {
          ...state,
          tags: data.tags || data, // Handle both {tags: [...]} and [...] responses
          isLoading: false
        };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('tags-loaded', { tags: state.tags });
        
        return state.tags;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch tags';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get a specific tag by ID
    getTag: async (id: string) => {
      // Check if we already have this tag
      const existingTag = state.tags.find(tag => tag.id === id);
      if (existingTag) {
        state = { ...state, currentTag: existingTag };
        return existingTag;
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.get(`${config.apiUrl}/tags/${id}`);
        const tag: Tag = await response.json();

        // Update tags list if this is a new tag
        const tagIndex = state.tags.findIndex(t => t.id === id);
        if (tagIndex === -1) {
          state.tags.push(tag);
        } else {
          state.tags[tagIndex] = tag;
        }

        state = {
          ...state,
          currentTag: tag,
          isLoading: false
        };

        return tag;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load tag';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new tag
    createTag: async (tagData: TagCreateRequest) => {
      // Validate color if provided
      if (tagData.color && !isValidHexColor(tagData.color)) {
        throw new Error('Invalid color format. Use hex color code (e.g., #FF0000)');
      }

      // Clean the data - remove empty strings
      const cleanedData = {
        ...tagData,
        description: tagData.description?.trim() || undefined
      };

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.post(`${config.apiUrl}/tags/`, cleanedData);
        
        const tag: Tag = await response.json();

        // Add to tags list
        state = {
          ...state,
          tags: [...state.tags, tag],
          currentTag: tag,
          isLoading: false
        };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('tag-created', { tag });

        return tag;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create tag';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new tag or get existing one by name (for inline tag creation)
    createOrGetTag: async (name: string, color?: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const params = new URLSearchParams({ name });
        if (color) {
          params.append('color', color);
        }

        const response = await api.get(`${config.apiUrl}/tags/create-or-get?${params}`);
        const data = await response.json();
        
        const tag: Tag = data.tag;
        const wasCreated: boolean = data.created;

        // Update tags list
        if (wasCreated) {
          // Add new tag to list
          state = {
            ...state,
            tags: [...state.tags, tag],
            currentTag: tag,
            isLoading: false
          };
          
          tagEventBus.emit('tag-created', { tag });
        } else {
          // Update existing tag if it was found
          const tagIndex = state.tags.findIndex(t => t.id === tag.id);
          if (tagIndex !== -1) {
            state.tags[tagIndex] = tag;
          } else {
            state.tags.push(tag);
          }
          
          state = {
            ...state,
            currentTag: tag,
            isLoading: false
          };
        }

        return { tag, wasCreated };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create or get tag';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Update an existing tag
    updateTag: async (id: string, tagData: TagUpdateRequest) => {
      // Validate color if provided
      if (tagData.color && !isValidHexColor(tagData.color)) {
        throw new Error('Invalid color format. Use hex color code (e.g., #FF0000)');
      }

      // Optimistic update
      const originalTag = state.tags.find(tag => tag.id === id);
      if (originalTag) {
        const optimisticTag = { ...originalTag, ...tagData, updated_at: new Date().toISOString() };
        state = {
          ...state,
          tags: state.tags.map(tag => tag.id === id ? optimisticTag : tag),
          currentTag: state.currentTag?.id === id ? optimisticTag : state.currentTag
        };
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.put(`${config.apiUrl}/tags/${id}`, tagData);
        
        const updatedTag: Tag = await response.json();

        state = {
          ...state,
          tags: state.tags.map(tag => tag.id === id ? updatedTag : tag),
          currentTag: state.currentTag?.id === id ? updatedTag : state.currentTag,
          isLoading: false
        };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('tag-updated', { tag: updatedTag, changes: tagData });

        return updatedTag;
      } catch (error) {
        // Rollback optimistic update
        if (originalTag) {
          state = {
            ...state,
            tags: state.tags.map(tag => tag.id === id ? originalTag : tag),
            currentTag: state.currentTag?.id === id ? originalTag : state.currentTag
          };
        }
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to update tag';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Delete a tag
    deleteTag: async (id: string) => {
      // Optimistic removal
      const originalTags = [...state.tags];
      const originalCurrentTag = state.currentTag;
      
      state = {
        ...state,
        tags: state.tags.filter(tag => tag.id !== id),
        currentTag: state.currentTag?.id === id ? null : state.currentTag
      };

      state = { ...state, isLoading: true, error: null };

      try {
        await api.delete(`${config.apiUrl}/tags/${id}`);

        state = { ...state, isLoading: false };
        
        // Remove from usage map
        state.tagUsageMap.delete(id);
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('tag-deleted', { tagId: id });
        
      } catch (error) {
        // Rollback optimistic update
        state = {
          ...state,
          tags: originalTags,
          currentTag: originalCurrentTag
        };
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete tag';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Assign tags to a user game
    assignTagsToGame: async (userGameId: string, tagIds: string[]) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.post(`${config.apiUrl}/tags/assign/${userGameId}`, { tag_ids: tagIds });
        
        const result = await response.json();
        
        // Update usage map
        tagIds.forEach(tagId => {
          const currentCount = state.tagUsageMap.get(tagId) || 0;
          state.tagUsageMap.set(tagId, currentCount + 1);
        });
        
        state = { ...state, isLoading: false };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('tags-assigned', { userGameId, tagIds, result });

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to assign tags';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Remove tags from a user game
    removeTagsFromGame: async (userGameId: string, tagIds: string[]) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.call(`${config.apiUrl}/tags/remove/${userGameId}`, {
          method: 'DELETE',
          body: JSON.stringify({ tag_ids: tagIds }),
        });
        
        const result = await response.json();
        
        // Update usage map
        tagIds.forEach(tagId => {
          const currentCount = state.tagUsageMap.get(tagId) || 0;
          if (currentCount > 0) {
            state.tagUsageMap.set(tagId, currentCount - 1);
          }
        });
        
        state = { ...state, isLoading: false };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('tags-removed', { userGameId, tagIds, result });

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove tags';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Bulk assign tags to multiple games
    bulkAssignTags: async (data: BulkTagAssignRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.post(`${config.apiUrl}/tags/bulk-assign`, data);
        
        const result = await response.json();
        
        // Update usage map (approximate - actual counts would need server data)
        data.tag_ids.forEach(tagId => {
          const currentCount = state.tagUsageMap.get(tagId) || 0;
          state.tagUsageMap.set(tagId, currentCount + data.user_game_ids.length);
        });
        
        state = { ...state, isLoading: false };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('bulk-tags-assigned', { data, result });

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk assign tags';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Bulk remove tags from multiple games
    bulkRemoveTags: async (data: BulkTagRemoveRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.call(`${config.apiUrl}/tags/bulk-remove`, {
          method: 'DELETE',
          body: JSON.stringify(data),
        });
        
        const result = await response.json();
        
        // Update usage map (approximate - actual counts would need server data)
        data.tag_ids.forEach(tagId => {
          const currentCount = state.tagUsageMap.get(tagId) || 0;
          const newCount = Math.max(0, currentCount - data.user_game_ids.length);
          state.tagUsageMap.set(tagId, newCount);
        });
        
        state = { ...state, isLoading: false };
        
        // Emit event for cross-view synchronization
        tagEventBus.emit('bulk-tags-removed', { data, result });

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk remove tags';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get tag usage statistics
    getTagUsageStats: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await api.get(`${config.apiUrl}/tags/usage/stats`);
        const stats = await response.json();
        
        // Update usage map
        state.tagUsageMap.clear();
        if (stats.tag_usage) {
          Object.entries(stats.tag_usage).forEach(([tagId, count]) => {
            state.tagUsageMap.set(tagId, count as number);
          });
        }
        
        // Update gameCount on tags
        state = {
          ...state,
          tags: state.tags.map(tag => ({
            ...tag,
            game_count: state.tagUsageMap.get(tag.id) || 0
          })),
          isLoading: false
        };
        
        return stats;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get tag usage stats';
        // Don't clear tags on error - preserve existing data
        state = { ...state, isLoading: false, error: errorMessage };
        log.warn('Tag usage stats failed, but preserving existing tags');
        // Don't throw - allow graceful degradation
      }
    },

    // Helper methods
    getTagById: (id: string) => {
      return state.tags.find(tag => tag.id === id);
    },

    getTagByName: (name: string) => {
      return state.tags.find(tag => tag.name.toLowerCase() === name.toLowerCase());
    },

    getTagsByIds: (ids: string[]) => {
      return state.tags.filter(tag => ids.includes(tag.id));
    },

    searchTags: (query: string) => {
      const lowerQuery = query.toLowerCase();
      return state.tags.filter(tag => 
        tag.name.toLowerCase().includes(lowerQuery) ||
        tag.description?.toLowerCase().includes(lowerQuery)
      );
    },

    getPopularTags: (limit: number = 10) => {
      return [...state.tags]
        .sort((a, b) => (b.game_count || 0) - (a.game_count || 0))
        .slice(0, limit);
    },

    getUnusedTags: () => {
      return state.tags.filter(tag => (tag.game_count || 0) === 0);
    },

    // State management
    clearCurrentTag: () => {
      state = { ...state, currentTag: null };
    },

    clearError: () => {
      state = { ...state, error: null };
    },

    // Validate tag name uniqueness (client-side)
    isTagNameUnique: (name: string, excludeId?: string) => {
      const normalizedName = name.trim().toLowerCase();
      return !state.tags.some(tag => 
        tag.name.toLowerCase() === normalizedName && 
        tag.id !== excludeId
      );
    },

    // Generate a suggested color for a new tag
    suggestColor: () => {
      // Find least used colors
      const usedColors = new Set(state.tags.map(tag => tag.color));
      const unusedColors = DEFAULT_TAG_COLORS.filter(color => !usedColors.has(color));
      
      if (unusedColors.length > 0) {
        // Return a random unused color
        return unusedColors[Math.floor(Math.random() * unusedColors.length)];
      }
      
      // If all colors are used, return a random one
      return DEFAULT_TAG_COLORS[Math.floor(Math.random() * DEFAULT_TAG_COLORS.length)];
    },

    // Event system integration
    on: tagEventBus.on.bind(tagEventBus),
    off: tagEventBus.off.bind(tagEventBus),
    emit: tagEventBus.emit.bind(tagEventBus),

    // Test helper - only use in tests
    __reset: () => {
      state = { ...initialState, tagUsageMap: new Map() };
    }
  };
}

export const tags = createTagsStore();