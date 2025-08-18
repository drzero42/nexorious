<script lang="ts">
  import { DEFAULT_TAG_COLORS } from '$lib/stores';

  interface Props {
    value: string;
    onchange: (color: string) => void;
    class?: string;
    disabled?: boolean;
  }

  let {
    value,
    onchange,
    class: className = '',
    disabled = false
  }: Props = $props();

  let showCustomInput = $state(false);
  let customColorInput = $state('');

  // Check if the current color is in the default palette
  const isCustomColor = $derived(!DEFAULT_TAG_COLORS.includes(value));

  const handleColorSelect = (color: string) => {
    if (disabled) return;
    
    onchange(color);
    showCustomInput = false;
    customColorInput = '';
  };

  const handleCustomColorSubmit = () => {
    const color = customColorInput.trim().toUpperCase();
    
    // Validate hex color format
    if (/^#[0-9A-F]{6}$/i.test(color)) {
      onchange(color);
      showCustomInput = false;
      customColorInput = '';
    }
  };

  const handleCustomColorKeydown = (event: KeyboardEvent) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      handleCustomColorSubmit();
    } else if (event.key === 'Escape') {
      showCustomInput = false;
      customColorInput = '';
    }
  };

  // Calculate text color for better contrast
  const getTextColor = (hexColor: string): string => {
    const hex = hexColor.replace('#', '');
    const r = parseInt(hex.substr(0, 2), 16);
    const g = parseInt(hex.substr(2, 2), 16);
    const b = parseInt(hex.substr(4, 2), 16);
    const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
    return luminance > 0.5 ? '#000000' : '#FFFFFF';
  };
</script>

<!-- 
ColorPicker Component
A color picker that shows predefined colors and allows custom hex input.

Props:
- value: Current selected color (hex format)
- onchange: Callback when color changes
- disabled: Whether the picker is disabled
-->

<div class="space-y-3 {className}">
  <!-- Current Color Display -->
  <div class="flex items-center gap-3">
    <div 
      class="
        h-8 w-8 rounded-lg border-2 border-gray-200 shadow-sm flex items-center justify-center
        {disabled ? 'opacity-50' : ''}
      "
      style="background-color: {value}; color: {getTextColor(value)}"
    >
      <span class="text-xs font-mono">
        {value.slice(1, 4).toUpperCase()}
      </span>
    </div>
    
    <div>
      <div class="text-sm font-medium text-gray-700">Selected Color</div>
      <div class="text-xs font-mono text-gray-500">{value}</div>
    </div>
  </div>

  <!-- Predefined Colors Grid -->
  <div>
    <div class="text-sm font-medium text-gray-700 mb-2">Choose a Color</div>
    <div class="grid grid-cols-6 gap-2">
      {#each DEFAULT_TAG_COLORS as color}
        <button
          type="button"
          class="
            h-8 w-8 rounded-lg border-2 transition-all duration-200
            hover:scale-110 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500
            {value === color ? 'border-gray-800 ring-2 ring-primary-500' : 'border-gray-200'}
            {disabled ? 'opacity-50 cursor-not-allowed' : 'hover:border-gray-400'}
          "
          style="background-color: {color}"
          aria-label="Select color {color}"
          title={color}
          {disabled}
          onclick={() => handleColorSelect(color)}
        >
          {#if value === color}
            <svg class="h-4 w-4 mx-auto" fill={getTextColor(color)} viewBox="0 0 20 20">
              <path
                fill-rule="evenodd"
                d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                clip-rule="evenodd"
              />
            </svg>
          {/if}
        </button>
      {/each}
    </div>
  </div>

  <!-- Custom Color Section -->
  <div>
    <div class="flex items-center justify-between mb-2">
      <div class="text-sm font-medium text-gray-700">Custom Color</div>
      {#if isCustomColor}
        <span class="text-xs text-blue-600 bg-blue-50 px-2 py-0.5 rounded">Custom</span>
      {/if}
    </div>
    
    {#if showCustomInput}
      <div class="flex gap-2">
        <div class="flex-1">
          <input
            type="text"
            class="
              block w-full rounded-md border-gray-300 shadow-sm text-sm font-mono
              focus:border-primary-500 focus:ring-primary-500
              {disabled ? 'bg-gray-50 text-gray-500' : ''}
            "
            placeholder="#FF0000"
            bind:value={customColorInput}
            onkeydown={handleCustomColorKeydown}
            {disabled}
            maxlength="7"
          />
        </div>
        <button
          type="button"
          class="
            px-3 py-2 bg-primary-600 text-white text-sm font-medium rounded-md
            hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500
            disabled:opacity-50 disabled:cursor-not-allowed
          "
          {disabled}
          onclick={handleCustomColorSubmit}
        >
          Apply
        </button>
        <button
          type="button"
          class="
            px-3 py-2 bg-gray-300 text-gray-700 text-sm font-medium rounded-md
            hover:bg-gray-400 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500
            disabled:opacity-50 disabled:cursor-not-allowed
          "
          {disabled}
          onclick={() => {
            showCustomInput = false;
            customColorInput = '';
          }}
        >
          Cancel
        </button>
      </div>
      <div class="mt-1 text-xs text-gray-500">
        Enter a hex color code (e.g., #FF0000 for red)
      </div>
    {:else}
      <button
        type="button"
        class="
          w-full px-3 py-2 border border-gray-300 rounded-md text-sm text-gray-700
          hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500
          disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-transparent
        "
        {disabled}
        onclick={() => {
          showCustomInput = true;
          customColorInput = value;
        }}
      >
        Enter Custom Color Code
      </button>
    {/if}
  </div>

  <!-- Color Accessibility Note -->
  <div class="text-xs text-gray-500 bg-gray-50 p-2 rounded">
    <strong>Note:</strong> Colors are automatically adjusted for text readability.
    Light colors use dark text, dark colors use light text.
  </div>
</div>