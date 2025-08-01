<script lang="ts">
 import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
 import { userGames } from '$lib/stores/user-games.svelte';
 import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';
 import { onMount, tick } from 'svelte';
 import Portal from './Portal.svelte';

 export let platforms: UserGamePlatform[] = [];
 export let compact: boolean = false;
 export let maxVisible: number = 3;
 export let showDetails: boolean = false; // For expanded detail view
 export let enableHover: boolean = true; // To control hover interactions
 export let showDetailedTooltips: boolean = true; // Enable rich tooltips
 export let showStoreLinks: boolean = false; // Include store links in tooltips
 // Note: tooltipPosition prop removed - now using automatic positioning

 // Tooltip state management
 let activeTooltip: string | null = null;
 let tooltipElements: { [key: string]: HTMLElement } = {};
 let isMobile = false;

// Hover delay management
let showTimeouts: { [key: string]: ReturnType<typeof setTimeout> } = {};
let hideTimeouts: { [key: string]: ReturnType<typeof setTimeout> } = {};
const SHOW_DELAY = 300; // ms delay before showing tooltip
const HIDE_DELAY = 100; // ms delay before hiding tooltip

// Tooltip positioning state for fixed positioning
let badgeElements: { [key: string]: HTMLElement } = {};
let tooltipCoords = { top: 0, left: 0 };
let tooltipPlacement: 'top' | 'bottom' | 'left' | 'right' = 'top';

 onMount(() => {
   // ===== COMPONENT INITIALIZATION DEBUG =====
   console.log('🔍 PlatformBadges: Component mounted with props:', {
     platforms: platforms?.length || 0,
     compact,
     maxVisible,
     showDetails,
     enableHover,
     showDetailedTooltips,
     showStoreLinks
   });
   
   // More accurate mobile detection - check for small screen size AND touch capability
   const hasTouch = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
   const isSmallScreen = window.innerWidth <= 768; // Common mobile breakpoint
   const isTouchPrimary = window.matchMedia('(pointer: coarse)').matches;
   
   // Consider it mobile if it has touch AND either small screen or coarse pointer
   isMobile = hasTouch && (isSmallScreen || isTouchPrimary);
   
   // ===== MOBILE DETECTION DEBUG =====
   console.log('📱 PlatformBadges: Mobile detection results:', {
     hasTouch,
     isSmallScreen,
     isTouchPrimary,
     isMobile,
     userAgent: navigator.userAgent
   });
 });

// Clear all timeouts on component cleanup
function clearAllTimeouts() {
  Object.values(showTimeouts).forEach(clearTimeout);
  Object.values(hideTimeouts).forEach(clearTimeout);
  showTimeouts = {};
  hideTimeouts = {};
}

onMount(() => {
  return () => {
    clearAllTimeouts();
  };
});

 $: groupedPlatforms = groupPlatformsByPlatform(platforms);
 $: visiblePlatforms = groupedPlatforms.slice(0, maxVisible);
 $: hiddenCount = Math.max(0, groupedPlatforms.length - maxVisible);

 // ===== DATA STRUCTURE DEBUG =====
 $: {
   console.log('📊 PlatformBadges: Data processed:', {
     originalPlatforms: platforms?.length || 0,
     groupedPlatforms: groupedPlatforms?.length || 0,
     visiblePlatforms: visiblePlatforms?.length || 0,
     hiddenCount,
     firstGroupId: visiblePlatforms?.[0] ? `platform-${visiblePlatforms[0].platform.id}` : 'none'
   });
   
   if (visiblePlatforms?.length > 0) {
     console.log('🎯 PlatformBadges: First visible platform details:', {
       platform: visiblePlatforms[0]?.platform,
       storefronts: visiblePlatforms[0]?.storefronts?.length || 0
     });
   }
 }

 // ===== ACTIVE TOOLTIP STATE DEBUG =====
 $: {
   console.log('🎭 PlatformBadges: activeTooltip state changed:', {
     activeTooltip,
     showDetailedTooltips,
     shouldRenderTooltip: !!activeTooltip && showDetailedTooltips
   });
 }
 
 // Template condition verification
 $: {
   console.log('🔍 Template conditions check:', {
     showDetailedTooltips,
     activeTooltip,
     'should render for each groupId': visiblePlatforms?.map(group => ({
       groupId: `platform-${group.platform.id}`,
       matches: activeTooltip === `platform-${group.platform.id}`,
       shouldRender: showDetailedTooltips && activeTooltip === `platform-${group.platform.id}`
     }))
   });
 }

 // Enhanced platform-specific styling with better visual design
 function getPlatformStyle(platformName: string): { bg: string, border: string, text: string, icon: string } {
  const name = platformName.toLowerCase();
  
  if (name.includes('playstation')) {
    return {
      bg: 'bg-gradient-to-r from-blue-600 to-blue-700',
      border: 'border-blue-800',
      text: 'text-white',
      icon: '🎮'
    };
  }
  if (name.includes('xbox')) {
    return {
      bg: 'bg-gradient-to-r from-green-600 to-green-700', 
      border: 'border-green-800',
      text: 'text-white',
      icon: '🎮'
    };
  }
  if (name.includes('nintendo') || name.includes('switch')) {
    return {
      bg: 'bg-gradient-to-r from-red-600 to-red-700',
      border: 'border-red-800', 
      text: 'text-white',
      icon: '🎮'
    };
  }
  if (name.includes('pc') || name.includes('windows')) {
    return {
      bg: 'bg-gradient-to-r from-gray-700 to-gray-800',
      border: 'border-gray-900',
      text: 'text-white',
      icon: '💻'
    };
  }
  if (name.includes('ios')) {
    return {
      bg: 'bg-gradient-to-r from-gray-800 to-gray-900',
      border: 'border-black',
      text: 'text-white',
      icon: '📱'
    };
  }
  if (name.includes('android')) {
    return {
      bg: 'bg-gradient-to-r from-green-700 to-green-800',
      border: 'border-green-900',
      text: 'text-white',
      icon: '📱'
    };
  }
  
  // Default style
  return {
    bg: 'bg-gradient-to-r from-indigo-600 to-indigo-700',
    border: 'border-indigo-800',
    text: 'text-white',
    icon: '🎯'
  };
 }

 // Get storefront-specific styling and icons
 function getStorefrontIcon(storefrontName: string): string {
  const name = storefrontName?.toLowerCase() || '';
  if (name.includes('steam')) return '🔥';
  if (name.includes('epic')) return '🎮';
  if (name.includes('gog')) return '🏪';
  if (name.includes('playstation')) return '🎮';
  if (name.includes('microsoft') || name.includes('xbox')) return '🎮';
  if (name.includes('nintendo')) return '🎮';
  if (name.includes('app store') || name.includes('apple')) return '📱';
  if (name.includes('google play')) return '🤖';
  if (name.includes('physical')) return '📦';
  if (name.includes('humble')) return '🎁';
  if (name.includes('itch')) return '🕹️';
  if (name.includes('origin') || name.includes('ea')) return '🎮';
  return '🏪'; // Default store icon
 }

 // Generate tooltip text for accessibility and hover details
 function generateTooltipText(group: any): string {
  const platformName = group.platform.display_name;
  const storefrontNames = group.storefronts
   .map((sf: any) => sf.storefront?.display_name)
   .filter(Boolean)
   .join(', ');
  
  return storefrontNames 
   ? `${platformName} - Available on: ${storefrontNames}`
   : `${platformName} - No specific storefront`;
 }

 // Show detailed tooltip with delay
 function showTooltip(groupId: string, event?: Event) {
   // ===== EVENT FLOW DEBUG =====
   console.log('🎯 PlatformBadges: showTooltip called:', {
     groupId,
     hasEvent: !!event,
     eventType: event?.type,
     showDetailedTooltips,
     isMobile,
     currentActiveTooltip: activeTooltip
   });
   
   if (!showDetailedTooltips) {
     console.log('❌ PlatformBadges: showDetailedTooltips is false, aborting');
     return;
   }
   
   // Clear any existing hide timeout for this tooltip
   if (hideTimeouts[groupId]) {
     clearTimeout(hideTimeouts[groupId]);
     delete hideTimeouts[groupId];
     console.log('⏰ Cleared hide timeout for:', groupId);
   }
   
   // If tooltip is already active, no need to delay
   if (activeTooltip === groupId) {
     console.log('✅ Tooltip already active for:', groupId);
     return;
   }
   
   const previousActiveTooltip = activeTooltip;
   
   if (isMobile && event) {
     // On mobile, toggle tooltip immediately on tap
     event.preventDefault();
     event.stopPropagation();
     activeTooltip = activeTooltip === groupId ? null : groupId;
     console.log('📝 Mobile: activeTooltip changed immediately:', {
       from: previousActiveTooltip,
       to: activeTooltip
     });
   } else {
     // On desktop, show tooltip after delay
     if (showTimeouts[groupId]) {
       clearTimeout(showTimeouts[groupId]);
     }
     
     console.log('⏰ Setting show timeout for:', groupId, 'delay:', SHOW_DELAY);
     showTimeouts[groupId] = setTimeout(() => {
       activeTooltip = groupId;
       delete showTimeouts[groupId];
       console.log('📝 Desktop: activeTooltip changed after delay:', {
         from: previousActiveTooltip,
         to: activeTooltip,
         shouldShowTooltip: true
       });
     }, SHOW_DELAY);
   }
 }

 // Hide tooltip with delay
 function hideTooltip(groupId: string) {
   if (!showDetailedTooltips || isMobile) return;
   
   // Clear any pending show timeout for this tooltip
   if (showTimeouts[groupId]) {
     clearTimeout(showTimeouts[groupId]);
     delete showTimeouts[groupId];
     console.log('⏰ Cleared show timeout for:', groupId);
   }
   
   // Only hide if this tooltip is currently active
   if (activeTooltip !== groupId) {
     console.log('ℹ️ Tooltip not active for:', groupId, 'current:', activeTooltip);
     return;
   }
   
   console.log('⏰ Setting hide timeout for:', groupId, 'delay:', HIDE_DELAY);
   hideTimeouts[groupId] = setTimeout(() => {
     // Double-check tooltip is still the active one before hiding
     if (activeTooltip === groupId) {
       console.log('🚷 hideTooltip after delay:', groupId);
       activeTooltip = null;
     }
     delete hideTimeouts[groupId];
   }, HIDE_DELAY);
 }

 // Calculate tooltip position using getBoundingClientRect for fixed positioning
function calculateTooltipPosition(badgeElement: HTMLElement, tooltipWidth: number = 256, tooltipHeight: number = 200): { top: number, left: number, placement: 'top' | 'bottom' | 'left' | 'right' } {
 const badgeRect = badgeElement.getBoundingClientRect();
 const viewportWidth = window.innerWidth;
 const viewportHeight = window.innerHeight;
 const scrollX = window.scrollX;
 const scrollY = window.scrollY;
 
 // Calculate potential positions
 const positions = {
  top: {
   top: badgeRect.top + scrollY - tooltipHeight - 8,
   left: badgeRect.left + scrollX + (badgeRect.width / 2) - (tooltipWidth / 2),
   placement: 'top' as const
  },
  bottom: {
   top: badgeRect.bottom + scrollY + 8,
   left: badgeRect.left + scrollX + (badgeRect.width / 2) - (tooltipWidth / 2),
   placement: 'bottom' as const
  },
  left: {
   top: badgeRect.top + scrollY + (badgeRect.height / 2) - (tooltipHeight / 2),
   left: badgeRect.left + scrollX - tooltipWidth - 8,
   placement: 'left' as const
  },
  right: {
   top: badgeRect.top + scrollY + (badgeRect.height / 2) - (tooltipHeight / 2),
   left: badgeRect.right + scrollX + 8,
   placement: 'right' as const
  }
 };
 
 // Check which positions fit in viewport (prefer top, then bottom, then sides)
 const preferredOrder: ('top' | 'bottom' | 'left' | 'right')[] = ['top', 'bottom', 'left', 'right'];
 
 for (const placement of preferredOrder) {
  const pos = positions[placement];
  const fitsHorizontally = pos.left >= 0 && pos.left + tooltipWidth <= viewportWidth;
  const fitsVertically = pos.top >= 0 && pos.top + tooltipHeight <= viewportHeight;
  
  if (fitsHorizontally && fitsVertically) {
   return pos;
  }
 }
 
 // If nothing fits perfectly, use top and clamp to viewport
 const fallbackPos = positions.top;
 return {
  top: Math.max(0, Math.min(fallbackPos.top, viewportHeight - tooltipHeight)),
  left: Math.max(0, Math.min(fallbackPos.left, viewportWidth - tooltipWidth)),
  placement: 'top'
 };
}

// Update tooltip position when badge is hovered/clicked
function updateTooltipPosition(groupId: string) {
 // ===== ELEMENT BINDING & POSITIONING DEBUG =====
 const badgeElement = badgeElements[groupId];
 console.log('📍 PlatformBadges: updateTooltipPosition called:', {
   groupId,
   hasBadgeElement: !!badgeElement,
   badgeElementKeys: Object.keys(badgeElements),
   activeTooltip
 });
 
 if (!badgeElement) {
   console.error('❌ PlatformBadges: Badge element not found for groupId:', groupId);
   return;
 }
 
 console.log('📐 PlatformBadges: Badge element bounds:', badgeElement.getBoundingClientRect());
 
 const position = calculateTooltipPosition(badgeElement);
 const previousCoords = { ...tooltipCoords };
 tooltipCoords = { top: position.top, left: position.left };
 tooltipPlacement = position.placement;
 
 console.log('🎯 PlatformBadges: Tooltip position calculated:', {
   previousCoords,
   newCoords: tooltipCoords,
   placement: tooltipPlacement,
   viewport: {
     width: window.innerWidth,
     height: window.innerHeight
   }
 });
}

// Handle positioning after Portal is ready
async function handlePortalReady(groupId: string) {
  await tick(); // Wait for DOM updates
  console.log('🚪 Portal ready for:', groupId, 'recalculating position...');
  updateTooltipPosition(groupId);
}


// Handle click events
 function handleClick(groupId: string, event: Event) {
   if (!showDetailedTooltips) return;
   
   if (isMobile) {
     showTooltip(groupId, event);
   } else {
     // On desktop, clicking toggles persistent tooltip
     event.preventDefault();
     event.stopPropagation();
     activeTooltip = activeTooltip === groupId ? null : groupId;
   }
 }

 // Handle keyboard interactions
 function handleKeydown(groupId: string, event: KeyboardEvent) {
   if (!showDetailedTooltips) return;
   
   if (event.key === 'Enter' || event.key === ' ') {
     event.preventDefault();
     activeTooltip = activeTooltip === groupId ? null : groupId;
   } else if (event.key === 'Escape') {
     activeTooltip = null;
   }
 }

 // Close tooltip when clicking outside
 function handleDocumentClick(event: Event) {
   if (!activeTooltip) return;
   
   const target = event.target as Element;
   const tooltipElement = tooltipElements[activeTooltip];
   
   if (tooltipElement && !tooltipElement.contains(target)) {
     activeTooltip = null;
   }
 }

 onMount(() => {
   document.addEventListener('click', handleDocumentClick);
   return () => {
     document.removeEventListener('click', handleDocumentClick);
   };
 });
</script>

{#if groupedPlatforms.length > 0}
 <div class="flex flex-wrap {compact ? 'gap-1.5' : 'gap-2 sm:gap-2.5'}">
  {#each visiblePlatforms as group (group.platform.id)}
   {@const platformStyle = getPlatformStyle(group.platform.display_name)}
   {@const groupId = `platform-${group.platform.id}`}
   <div 
        class="relative inline-flex items-center rounded-lg transition-all duration-200 border-2 shadow-lg
               {platformStyle.bg} {platformStyle.border} {platformStyle.text}
               {compact 
                 ? 'px-2.5 py-1.5 min-h-[32px]' 
                 : 'px-3 py-2 min-h-[44px] sm:px-4 sm:py-2.5 sm:min-h-[40px]'}
               {enableHover ? 'active:scale-95 sm:hover:scale-105 sm:hover:shadow-xl' : ''}
               {showDetailedTooltips ? 'cursor-pointer' : 'cursor-help'}" 
        role="button" 
        tabindex="0"
        title="{showDetailedTooltips ? 'Click for details' : generateTooltipText(group)}"
        aria-label="{showDetailedTooltips ? 'Show platform details for ' + group.platform.display_name : generateTooltipText(group)}"
        bind:this={badgeElements[groupId]}
        on:mouseenter={() => {
          showTooltip(groupId);
          if (activeTooltip === groupId) {
            updateTooltipPosition(groupId);
          }
        }}
        on:mouseleave={() => {
          hideTooltip(groupId);
        }}
        on:click={(e) => {
          handleClick(groupId, e);
          if (activeTooltip === groupId) {
            updateTooltipPosition(groupId);
          }
        }}
        on:keydown={(e) => handleKeydown(groupId, e)}
   >
    
    <!-- Platform Icon and Name -->
    <div class="flex items-center {compact ? 'gap-1.5' : 'gap-2'}">
     <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'}" role="img" aria-hidden="true">
      {platformStyle.icon}
     </span>
     <span class="font-bold {compact ? 'text-xs' : 'text-sm'} tracking-wide {compact ? '' : 'truncate max-w-[80px] sm:max-w-none'}">
      {group.platform.display_name}
     </span>
    </div>
    
    <!-- Storefronts - responsive display -->
    {#if group.storefronts.length > 0 && showDetails}
     <span class="mx-2 opacity-80 font-bold {compact ? 'mx-1.5 text-xs' : 'text-sm'}">•</span>
     <div class="flex items-center flex-wrap {compact ? 'gap-1' : 'gap-1.5'}">
      {#each group.storefronts as storefront, index (storefront.id)}
       <div class="inline-flex items-center {compact ? 'gap-1' : 'gap-1.5'}">
        <span class="{compact ? 'text-xs' : 'text-sm'}" role="img" aria-hidden="true">
         {getStorefrontIcon(storefront.storefront?.display_name || '')}
        </span>
        <span class="font-medium {compact ? 'text-xs' : 'text-sm'} 
                     bg-white bg-opacity-20 px-2 py-0.5 rounded-md
                     {compact ? 'px-1.5 py-0.5' : 'px-2 py-1'}" 
              title="{storefront.storefront?.display_name}">
         {storefront.storefront?.display_name}
        </span>
       </div>
       {#if index < group.storefronts.length - 1}
        <span class="opacity-60 {compact ? 'text-xs' : 'text-sm'} mx-0.5">•</span>
       {/if}
      {/each}
     </div>
    {:else if group.storefronts.length > 0}
     <!-- Compact storefront indicators -->
     <div class="ml-2 flex items-center {compact ? 'gap-1' : 'gap-1.5'}">
      <span class="opacity-80 {compact ? 'text-xs' : 'text-sm'}">•</span>
      <!-- On mobile show just count, on desktop show icons -->
      <div class="sm:hidden">
       <span class="inline-flex items-center justify-center w-5 h-5 bg-white bg-opacity-30 rounded-full text-xs font-bold">
        {group.storefronts.length}
       </span>
      </div>
      <div class="hidden sm:flex items-center -space-x-1">
       {#each group.storefronts.slice(0, 3) as storefront}
        <span class="inline-flex items-center justify-center w-5 h-5 bg-white bg-opacity-30 rounded-full text-xs" 
              title="{storefront.storefront?.display_name}">
         {getStorefrontIcon(storefront.storefront?.display_name || '')}
        </span>
       {/each}
       {#if group.storefronts.length > 3}
        <span class="inline-flex items-center justify-center w-5 h-5 bg-white bg-opacity-30 rounded-full text-xs font-bold">
         +{group.storefronts.length - 3}
        </span>
       {/if}
      </div>
     </div>
    {/if}
    
    <!-- Detailed Tooltip -->
    {#if showDetailedTooltips && activeTooltip === groupId && !userGames?.value?.isLoading}
     <Portal onPortalReady={() => handlePortalReady(groupId)}>
      <div 
           class="fixed z-[9999] w-64 p-3 bg-white border border-gray-300 rounded-lg shadow-xl"
           style="top: {tooltipCoords.top}px; left: {tooltipCoords.left}px;"
           role="tooltip"
           aria-labelledby="tooltip-{groupId}"
           data-tooltip-id="{groupId}"
           data-tooltip-coords="{tooltipCoords.top},{tooltipCoords.left}"
           bind:this={tooltipElements[groupId]}
           on:mouseenter={() => {
             // Cancel any pending hide timeout when hovering over tooltip
             if (hideTimeouts[groupId]) {
               clearTimeout(hideTimeouts[groupId]);
               delete hideTimeouts[groupId];
               console.log('⏰ Tooltip hover: cleared hide timeout for:', groupId);
             }
           }}
           on:mouseleave={() => {
             // Start hide timeout when leaving tooltip
             hideTooltip(groupId);
           }}
      >
      <!-- Arrow -->
      <div class="absolute left-1/2 transform -translate-x-1/2 w-3 h-3 bg-white border-l border-t border-gray-300 rotate-45
                  {tooltipPlacement === 'top' ? 'top-full -mt-1.5' : tooltipPlacement === 'bottom' ? 'bottom-full -mb-1.5' : tooltipPlacement === 'left' ? 'right-full -mr-1.5 top-1/2 -translate-y-1/2' : tooltipPlacement === 'right' ? 'left-full -ml-1.5 top-1/2 -translate-y-1/2' : 'top-full -mt-1.5'}"></div>
      
      <!-- Platform Header -->
      <div class="flex items-center gap-2 mb-3 pb-2 border-b border-gray-200">
       <span class="text-lg" role="img" aria-hidden="true">{platformStyle.icon}</span>
       <h4 class="font-semibold text-gray-900 text-sm" id="tooltip-{groupId}">
        {group.platform.display_name}
       </h4>
      </div>
      
      <!-- Storefronts List -->
      {#if group.storefronts.length > 0}
       <div class="space-y-2">
        <h5 class="text-xs font-medium text-gray-700 uppercase tracking-wide">Available On:</h5>
        <div class="space-y-1.5">
         {#each group.storefronts as storefront}
          <div class="flex items-center justify-between gap-2 text-sm">
           <div class="flex items-center gap-2 flex-1 min-w-0">
            <span class="text-sm" role="img" aria-hidden="true">
             {getStorefrontIcon(storefront.storefront?.display_name || '')}
            </span>
            <span class="text-gray-800 truncate">
             {storefront.storefront?.display_name || 'Unknown Store'}
            </span>
           </div>
           {#if showStoreLinks && storefront.store_url && storefront.storefront?.name !== 'physical'}
            <a 
                 href={storefront.store_url} 
                 target="_blank" 
                 rel="noopener noreferrer"
                 class="flex-shrink-0 text-blue-600 hover:text-blue-800 transition-colors"
                 title="Open in {storefront.storefront?.display_name}"
                 aria-label="Open {storefront.storefront?.display_name} store page"
                 on:click|stopPropagation
            >
             <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
             </svg>
            </a>
           {/if}
          </div>
         {/each}
        </div>
       </div>
      {:else}
       <div class="text-sm text-gray-500 italic">
        No specific storefront
       </div>
      {/if}
      
      <!-- Instructions -->
      {#if !isMobile}
       <div class="mt-3 pt-2 border-t border-gray-200 text-xs text-gray-500">
        Click to keep open • Hover to show
       </div>
      {/if}
      </div>
     </Portal>
    {/if}
   </div>
  {/each}
  
  {#if hiddenCount > 0}
   {@const hiddenGroupId = 'hidden-platforms'}
   <div class="relative inline-flex items-center rounded-lg transition-all duration-200 border-2 shadow-lg
               bg-gradient-to-r from-gray-600 to-gray-700 border-gray-800 text-white
               {compact 
                 ? 'px-2.5 py-1.5 min-h-[32px]' 
                 : 'px-3 py-2 min-h-[44px] sm:px-4 sm:py-2.5 sm:min-h-[40px]'}
               {enableHover ? 'active:scale-95 sm:hover:scale-105 sm:hover:shadow-xl' : ''}
               {showDetailedTooltips ? 'cursor-pointer' : 'cursor-help'}"
        role="button"
        tabindex="0"
        title="{showDetailedTooltips ? 'Click to see more platforms' : 'There are ' + hiddenCount + ' more platform' + (hiddenCount !== 1 ? 's' : '') + ' not shown'}"
        aria-label="{showDetailedTooltips ? 'Show ' + hiddenCount + ' more platforms' : 'There are ' + hiddenCount + ' more platform' + (hiddenCount !== 1 ? 's' : '') + ' not shown'}"
        bind:this={badgeElements[hiddenGroupId]}
        on:mouseenter={() => {
          showTooltip(hiddenGroupId);
          if (activeTooltip === hiddenGroupId) {
            updateTooltipPosition(hiddenGroupId);
          }
        }}
        on:mouseleave={() => {
          hideTooltip(hiddenGroupId);
        }}
        on:click={(e) => {
          handleClick(hiddenGroupId, e);
          if (activeTooltip === hiddenGroupId) {
            updateTooltipPosition(hiddenGroupId);
          }
        }}
        on:keydown={(e) => handleKeydown(hiddenGroupId, e)}>
    <div class="flex items-center {compact ? 'gap-1.5' : 'gap-2'}">
     <span class="{compact ? 'text-xs sm:text-sm' : 'text-sm sm:text-base'}" role="img" aria-hidden="true">📦</span>
     <span class="{compact ? 'text-xs' : 'text-sm'} font-bold">+{hiddenCount} {compact ? '' : 'more'}</span>
    </div>
    
    <!-- Hidden Platforms Tooltip -->
    {#if showDetailedTooltips && activeTooltip === hiddenGroupId && !userGames?.value?.isLoading}
     <Portal onPortalReady={() => handlePortalReady(hiddenGroupId)}>
      {@const hiddenPlatforms = groupedPlatforms.slice(maxVisible)}
      <div 
           class="fixed z-[9999] w-72 p-3 bg-white border border-gray-300 rounded-lg shadow-xl"
           style="top: {tooltipCoords.top}px; left: {tooltipCoords.left}px;"
           role="tooltip"
           aria-labelledby="tooltip-{hiddenGroupId}"
           data-tooltip-id="{hiddenGroupId}"
           data-tooltip-coords="{tooltipCoords.top},{tooltipCoords.left}"
           bind:this={tooltipElements[hiddenGroupId]}
           on:mouseenter={() => {
             // Cancel any pending hide timeout when hovering over tooltip
             if (hideTimeouts[hiddenGroupId]) {
               clearTimeout(hideTimeouts[hiddenGroupId]);
               delete hideTimeouts[hiddenGroupId];
               console.log('⏰ Hidden tooltip hover: cleared hide timeout for:', hiddenGroupId);
             }
           }}
           on:mouseleave={() => {
             // Start hide timeout when leaving tooltip
             hideTooltip(hiddenGroupId);
           }}>
      <!-- Arrow -->
      <div class="absolute left-1/2 transform -translate-x-1/2 w-3 h-3 bg-white border-l border-t border-gray-300 rotate-45
                  {tooltipPlacement === 'top' ? 'top-full -mt-1.5' : tooltipPlacement === 'bottom' ? 'bottom-full -mb-1.5' : tooltipPlacement === 'left' ? 'right-full -mr-1.5 top-1/2 -translate-y-1/2' : tooltipPlacement === 'right' ? 'left-full -ml-1.5 top-1/2 -translate-y-1/2' : 'top-full -mt-1.5'}"></div>
      
      <!-- Header -->
      <div class="flex items-center gap-2 mb-3 pb-2 border-b border-gray-200">
       <span class="text-lg" role="img" aria-hidden="true">📦</span>
       <h4 class="font-semibold text-gray-900 text-sm" id="tooltip-{hiddenGroupId}">
        Additional Platforms ({hiddenCount})
       </h4>
      </div>
      
      <!-- Hidden Platforms List -->
      <div class="space-y-3 max-h-48 overflow-y-auto">
       {#each hiddenPlatforms as group}
        {@const platformStyle = getPlatformStyle(group.platform.display_name)}
        <div class="flex items-start gap-3">
         <span class="text-sm flex-shrink-0 mt-0.5" role="img" aria-hidden="true">{platformStyle.icon}</span>
         <div class="flex-1 min-w-0">
          <div class="font-medium text-gray-900 text-sm truncate">{group.platform.display_name}</div>
          {#if group.storefronts.length > 0}
           <div class="mt-1 space-y-1">
            {#each group.storefronts as storefront}
             <div class="flex items-center gap-2 text-xs text-gray-600">
              <span role="img" aria-hidden="true">{getStorefrontIcon(storefront.storefront?.display_name || '')}</span>
              <span class="truncate">{storefront.storefront?.display_name || 'Unknown Store'}</span>
              {#if showStoreLinks && storefront.store_url && storefront.storefront?.name !== 'physical'}
               <a 
                    href={storefront.store_url} 
                    target="_blank" 
                    rel="noopener noreferrer"
                    class="flex-shrink-0 text-blue-600 hover:text-blue-800 transition-colors ml-auto"
                    title="Open in {storefront.storefront?.display_name}"
                    aria-label="Open {storefront.storefront?.display_name} store page"
                    on:click|stopPropagation>
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                 <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                </svg>
               </a>
              {/if}
             </div>
            {/each}
           </div>
          {:else}
           <div class="text-xs text-gray-500 italic mt-1">No specific storefront</div>
          {/if}
         </div>
        </div>
       {/each}
      </div>
      
      <!-- Instructions -->
      {#if !isMobile}
       <div class="mt-3 pt-2 border-t border-gray-200 text-xs text-gray-500">
        Click to keep open • Hover to show
       </div>
      {/if}
      </div>
     </Portal>
    {/if}
   </div>
  {/if}
 </div>
{:else}
 <div class="inline-flex items-center {compact ? 'px-2.5 py-1.5' : 'px-3 py-2 sm:px-4 sm:py-2.5'} {compact ? 'min-h-[32px]' : 'min-h-[44px] sm:min-h-[40px]'} rounded-lg bg-gray-100 border border-gray-200">
  <span class="text-gray-500 {compact ? 'text-xs' : 'text-sm'} italic font-medium flex items-center gap-2" 
        role="status" 
        aria-label="No platforms available for this game">
   <span role="img" aria-hidden="true">❌</span>
   No platforms
  </span>
 </div>
{/if}