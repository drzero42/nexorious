<script lang="ts">
  import { tags, type Tag } from '$lib/stores';
  import TagBadge from './TagBadge.svelte';
  
  interface Props {
    selectedTags?: Tag[];
    placeholder?: string;
    disabled?: boolean;
    maxTags?: number;
    allowCreate?: boolean;
    class?: string;
    onadd?: (tag: Tag) => void;
    onremove?: (tag: Tag) => void;
    onchange?: (selectedTags: Tag[]) => void;
  }

  let {
    selectedTags = [],
    placeholder = 'Type to search or create tags...',
    disabled = false,
    maxTags,
    allowCreate = true,
    class: className = '',
    onadd,
    onremove,
    onchange
  }: Props = $props();

  let inputValue = $state('');
  let showSuggestions = $state(false);
  let highlightedIndex = $state(-1);
  let inputRef: HTMLInputElement;

  // Get available tags (not already selected)
  const availableTags = $derived(() => {
    const selectedTagIds = new Set(selectedTags.map(tag => tag.id));
    return tags.value.tags.filter(tag => !selectedTagIds.has(tag.id));
  });

  // Filter suggestions based on input
  const suggestions = $derived(() => {
    if (!inputValue.trim()) return [];
    
    const query = inputValue.toLowerCase().trim();
    const filtered = availableTags().filter(tag => 
      tag.name.toLowerCase().includes(query) ||
      tag.description?.toLowerCase().includes(query)
    );
    
    // Sort by relevance (exact match first, then starts with, then contains)
    return filtered.sort((a, b) => {
      const aName = a.name.toLowerCase();
      const bName = b.name.toLowerCase();
      
      if (aName === query) return -1;
      if (bName === query) return 1;
      if (aName.startsWith(query) && !bName.startsWith(query)) return -1;
      if (bName.startsWith(query) && !aName.startsWith(query)) return 1;
      
      return aName.localeCompare(bName);
    });
  });

  // Check if we can create a new tag
  const canCreateNewTag = $derived(() => {
    if (!allowCreate || !inputValue.trim()) return false;
    
    const query = inputValue.toLowerCase().trim();
    const existingTag = tags.value.tags.find(tag => tag.name.toLowerCase() === query);
    
    return !existingTag && query.length > 0 && query.length <= 100;
  });

  // Check if max tags reached
  const maxTagsReached = $derived(() => {
    return maxTags !== undefined && selectedTags.length >= maxTags;
  });

  const handleInputChange = (event: Event) => {
    const target = event.target as HTMLInputElement;
    inputValue = target.value;
    showSuggestions = true;
    highlightedIndex = -1;
  };

  const handleInputKeydown = (event: KeyboardEvent) => {
    if (disabled) return;

    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        const maxIndex = canCreateNewTag() ? suggestions().length : suggestions().length - 1;
        highlightedIndex = Math.min(highlightedIndex + 1, maxIndex);
        break;
        
      case 'ArrowUp':
        event.preventDefault();
        highlightedIndex = Math.max(highlightedIndex - 1, -1);
        break;
        
      case 'Enter':
        event.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < suggestions().length) {
          // Select highlighted suggestion
          const selectedTag = suggestions()[highlightedIndex];
          if (selectedTag) {
            addTag(selectedTag);
          }
        } else if (highlightedIndex === suggestions().length && canCreateNewTag()) {
          // Create new tag
          createAndAddTag(inputValue.trim());
        } else if (suggestions().length === 1) {
          // Auto-select single suggestion
          const firstTag = suggestions()[0];
          if (firstTag) {
            addTag(firstTag);
          }
        } else if (canCreateNewTag() && suggestions().length === 0) {
          // Create new tag if no suggestions
          createAndAddTag(inputValue.trim());
        }
        break;
        
      case 'Escape':
        showSuggestions = false;
        highlightedIndex = -1;
        inputRef?.blur();
        break;
        
      case 'Backspace':
        if (inputValue === '' && selectedTags.length > 0) {
          // Remove last selected tag
          const lastTag = selectedTags[selectedTags.length - 1];
          if (lastTag) {
            removeTag(lastTag);
          }
        }
        break;
    }
  };

  const addTag = (tag: Tag) => {
    if (maxTagsReached()) return;
    
    const newSelectedTags = [...selectedTags, tag];
    
    if (onadd) onadd(tag);
    if (onchange) onchange(newSelectedTags);
    
    // Reset input
    inputValue = '';
    showSuggestions = false;
    highlightedIndex = -1;
  };

  const removeTag = (tag: Tag) => {
    const newSelectedTags = selectedTags.filter(t => t.id !== tag.id);
    
    if (onremove) onremove(tag);
    if (onchange) onchange(newSelectedTags);
  };

  const createAndAddTag = async (name: string) => {
    if (!allowCreate || maxTagsReached()) return;
    
    try {
      const suggestedColor = tags.suggestColor();
      const { tag } = await tags.createOrGetTag(name, suggestedColor);
      addTag(tag);
    } catch (error) {
      console.error('Failed to create tag:', error);
      // You might want to show a toast notification here
    }
  };

  const handleSuggestionClick = (tag: Tag) => {
    addTag(tag);
  };

  const handleCreateClick = () => {
    if (canCreateNewTag()) {
      createAndAddTag(inputValue.trim());
    }
  };

  const handleFocus = () => {
    if (!disabled) {
      showSuggestions = true;
    }
  };

  const handleBlur = () => {
    // Delay hiding suggestions to allow for clicks
    setTimeout(() => {
      showSuggestions = false;
      highlightedIndex = -1;
    }, 200);
  };
</script>

<!-- 
TagInput Component
An input field with autocomplete for selecting and creating tags.

Props:
- selectedTags: Currently selected tags
- placeholder: Input placeholder text
- disabled: Whether the input is disabled
- maxTags: Maximum number of tags allowed
- allowCreate: Whether to allow creating new tags
- onadd: Callback when a tag is added
- onremove: Callback when a tag is removed  
- onchange: Callback when the selection changes
-->

<div class="relative {className}">
  <!-- Selected Tags Display -->
  {#if selectedTags.length > 0}
    <div class="flex flex-wrap gap-1 mb-2">
      {#each selectedTags as tag (tag.id)}
        <TagBadge
          {tag}
          removable
          size="sm"
          onremove={removeTag}
        />
      {/each}
    </div>
  {/if}

  <!-- Input Field -->
  <div class="relative">
    <input
      bind:this={inputRef}
      type="text"
      class="
        block w-full rounded-md border-gray-300 shadow-sm text-sm
        focus:border-primary-500 focus:ring-primary-500
        {disabled ? 'bg-gray-50 text-gray-500 cursor-not-allowed' : ''}
        {maxTagsReached() ? 'bg-gray-50 text-gray-400' : ''}
      "
      {placeholder}
      bind:value={inputValue}
      {disabled}
      readonly={maxTagsReached()}
      oninput={handleInputChange}
      onkeydown={handleInputKeydown}
      onfocus={handleFocus}
      onblur={handleBlur}
      autocomplete="off"
      role="combobox"
      aria-expanded={showSuggestions}
      aria-controls="tag-suggestions-listbox"
      aria-haspopup="listbox"
      aria-label="Tag input with autocomplete"
    />
    
    {#if maxTagsReached()}
      <div class="absolute inset-y-0 right-0 flex items-center pr-3">
        <span class="text-xs text-gray-400">
          Max {maxTags} tags
        </span>
      </div>
    {/if}
  </div>

  <!-- Suggestions Dropdown -->
  {#if showSuggestions && !disabled && !maxTagsReached() && (suggestions().length > 0 || canCreateNewTag())}
    <div class="
      absolute z-50 mt-1 w-full bg-white border border-gray-300 rounded-md shadow-lg
      max-h-60 overflow-auto
    ">
      <ul id="tag-suggestions-listbox" role="listbox" class="py-1">
        <!-- Existing tag suggestions -->
        {#each suggestions() as tag, index (tag.id)}
          <li
            class="
              px-3 py-2 cursor-pointer flex items-center justify-between
              {index === highlightedIndex ? 'bg-primary-100 text-primary-900' : 'text-gray-900 hover:bg-gray-100'}
            "
            role="option"
            aria-selected={index === highlightedIndex}
            tabindex="-1"
            onclick={() => handleSuggestionClick(tag)}
            onkeydown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                handleSuggestionClick(tag);
              }
            }}
          >
            <div class="flex items-center gap-2">
              <div 
                class="w-3 h-3 rounded-full border border-gray-300"
                style="background-color: {tag.color}"
              ></div>
              <span class="font-medium">{tag.name}</span>
              {#if tag.description}
                <span class="text-sm text-gray-500">- {tag.description}</span>
              {/if}
            </div>
            {#if tag.game_count !== undefined && tag.game_count > 0}
              <span class="text-xs text-gray-400">
                {tag.game_count} game{tag.game_count !== 1 ? 's' : ''}
              </span>
            {/if}
          </li>
        {/each}

        <!-- Create new tag option -->
        {#if canCreateNewTag()}
          <li
            class="
              px-3 py-2 cursor-pointer flex items-center gap-2 border-t border-gray-200
              {highlightedIndex === suggestions().length ? 'bg-primary-100 text-primary-900' : 'text-gray-900 hover:bg-gray-100'}
            "
            role="option"
            aria-selected={highlightedIndex === suggestions().length}
            tabindex="-1"
            onclick={handleCreateClick}
            onkeydown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                handleCreateClick();
              }
            }}
          >
            <div class="w-3 h-3 rounded-full bg-gray-300 flex items-center justify-center">
              <svg class="w-2 h-2 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"/>
              </svg>
            </div>
            <span>
              Create "<strong>{inputValue.trim()}</strong>"
            </span>
          </li>
        {/if}
      </ul>
    </div>
  {/if}

  <!-- No suggestions message -->
  {#if showSuggestions && !disabled && !maxTagsReached && inputValue.trim() && suggestions.length === 0 && !canCreateNewTag}
    <div class="
      absolute z-50 mt-1 w-full bg-white border border-gray-300 rounded-md shadow-lg
    ">
      <div class="px-3 py-2 text-sm text-gray-500">
        No tags found matching "{inputValue.trim()}"
      </div>
    </div>
  {/if}

  <!-- Helper text -->
  <div class="mt-1 text-xs text-gray-500">
    {#if maxTags}
      {selectedTags.length} / {maxTags} tags selected
    {/if}
    {#if allowCreate}
      • Press Enter to create new tags
    {/if}
    {#if selectedTags.length > 0}
      • Press Backspace to remove the last tag
    {/if}
  </div>
</div>

<style>
  /* Ensure dropdown appears above other elements */
  .relative {
    position: relative;
  }
</style>