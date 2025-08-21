<script lang="ts">
  import type { PlatformSuggestion } from '$lib/types/platform-resolution';

  interface Props {
    suggestion: PlatformSuggestion;
    onUseSuggestion: () => void;
    disabled?: boolean;
  }

  let { 
    suggestion, 
    onUseSuggestion, 
    disabled = false 
  }: Props = $props();

  // Confidence level derived from confidence score
  const confidenceLevel = $derived(
    suggestion.confidence >= 0.8 ? 'high' : 
    suggestion.confidence >= 0.6 ? 'medium' : 'low'
  );

  // Color classes based on confidence level
  const confidenceColors = $derived(
    confidenceLevel === 'high' 
      ? { bg: 'bg-green-100', text: 'text-green-800', border: 'border-green-200' }
      : confidenceLevel === 'medium'
      ? { bg: 'bg-yellow-100', text: 'text-yellow-800', border: 'border-yellow-200' }
      : { bg: 'bg-red-100', text: 'text-red-800', border: 'border-red-200' }
  );

  // Progress bar width as percentage
  const progressWidth = $derived(Math.round(suggestion.confidence * 100));

  // Match type display
  const matchTypeDisplay = $derived(
    suggestion.match_type === 'exact' ? 'Exact Match' :
    suggestion.match_type === 'fuzzy' ? 'Similar Match' :
    'Partial Match'
  );

  // Match type icon
  const matchTypeIcon = $derived(
    suggestion.match_type === 'exact' ? '🎯' :
    suggestion.match_type === 'fuzzy' ? '🔍' :
    '📝'
  );

  function handleClick() {
    if (!disabled) {
      onUseSuggestion();
    }
  }
</script>

<div 
  class="flex items-center justify-between p-3 border rounded-lg hover:shadow-sm transition-all duration-200 {confidenceColors.border} {disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer hover:border-blue-300'}"
  onclick={handleClick}
  role="button"
  tabindex={disabled ? -1 : 0}
  onkeydown={(e) => {
    if ((e.key === 'Enter' || e.key === ' ') && !disabled) {
      e.preventDefault();
      onUseSuggestion();
    }
  }}
>
  <!-- Platform Info -->
  <div class="flex items-center space-x-3 flex-1">
    <!-- Platform Icon (if available) -->
    <div class="flex-shrink-0">
      <div class="w-8 h-8 rounded-full bg-gray-200 flex items-center justify-center text-sm">
        🎮
      </div>
    </div>

    <!-- Platform Details -->
    <div class="flex-1 min-w-0">
      <h5 class="text-sm font-medium text-gray-900 truncate">
        {suggestion.platform_display_name}
      </h5>
      <div class="flex items-center space-x-2 mt-1">
        <!-- Match Type Badge -->
        <span class="inline-flex items-center text-xs">
          <span class="mr-1">{matchTypeIcon}</span>
          {matchTypeDisplay}
        </span>
        
        <!-- Confidence Score -->
        <span class="text-xs text-gray-500">
          •
        </span>
        
        <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {confidenceColors.bg} {confidenceColors.text}">
          {progressWidth}% confidence
        </span>
      </div>
      
      <!-- Reason -->
      {#if suggestion.reason}
        <p class="text-xs text-gray-500 mt-1 line-clamp-2">
          {suggestion.reason}
        </p>
      {/if}
    </div>
  </div>

  <!-- Confidence Visualization -->
  <div class="flex items-center space-x-3 flex-shrink-0">
    <!-- Progress Bar -->
    <div class="w-16">
      <div class="bg-gray-200 rounded-full h-1.5">
        <div 
          class="h-1.5 rounded-full transition-all duration-300 {
            confidenceLevel === 'high' ? 'bg-green-500' :
            confidenceLevel === 'medium' ? 'bg-yellow-500' :
            'bg-red-500'
          }"
          style="width: {progressWidth}%"
        ></div>
      </div>
    </div>

    <!-- Use Button -->
    <button
      onclick={(e) => {
        e.stopPropagation();
        handleClick();
      }}
      {disabled}
      class="px-3 py-1.5 text-xs font-medium rounded-md border border-transparent text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
    >
      Use This
    </button>
  </div>
</div>

<!-- Additional Details (for high confidence matches) -->
{#if suggestion.confidence >= 0.85}
  <div class="mt-2 text-xs text-gray-600 bg-green-50 rounded-md p-2 border border-green-200">
    <div class="flex items-center">
      <svg class="w-3 h-3 text-green-500 mr-1" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"></path>
      </svg>
      <span class="font-medium text-green-800">Recommended:</span>
      <span class="ml-1 text-green-700">High confidence match</span>
    </div>
  </div>
{:else if suggestion.confidence < 0.6}
  <div class="mt-2 text-xs text-gray-600 bg-red-50 rounded-md p-2 border border-red-200">
    <div class="flex items-center">
      <svg class="w-3 h-3 text-red-500 mr-1" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd"></path>
      </svg>
      <span class="font-medium text-red-800">Caution:</span>
      <span class="ml-1 text-red-700">Low confidence match - verify carefully</span>
    </div>
  </div>
{/if}