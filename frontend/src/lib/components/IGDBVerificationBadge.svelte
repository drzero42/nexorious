<script lang="ts">
 export let isVerified: boolean = false;
 export let size: 'sm' | 'md' | 'lg' = 'sm';
 export let showTooltip: boolean = true;
 export let className: string = '';

 // Size-specific styling
 $: sizeClasses = {
   sm: 'w-4 h-4 text-xs',
   md: 'w-5 h-5 text-sm', 
   lg: 'w-6 h-6 text-base'
 }[size];

 $: badgeClasses = isVerified 
   ? 'bg-gradient-to-r from-blue-500 to-blue-600 text-white border-blue-700' 
   : 'bg-gray-100 text-gray-400 border-gray-300';

 $: tooltipText = isVerified 
   ? 'Verified from IGDB database - metadata is official and up-to-date'
   : 'Not verified from IGDB - metadata may be manually entered';
</script>

{#if isVerified}
<!-- Verified Badge -->
<div 
  class="inline-flex items-center justify-center rounded-full border shadow-sm transition-all duration-200 
         hover:shadow-md {sizeClasses} {badgeClasses} {className}"
  title={showTooltip ? tooltipText : ''}
  aria-label={tooltipText}
  role="img"
>
  <!-- Checkmark SVG Icon -->
  <svg 
    class="w-3 h-3" 
    fill="currentColor" 
    viewBox="0 0 20 20" 
    xmlns="http://www.w3.org/2000/svg"
    aria-hidden="true"
  >
    <path 
      fill-rule="evenodd" 
      d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" 
      clip-rule="evenodd"
    />
  </svg>
</div>
{/if}