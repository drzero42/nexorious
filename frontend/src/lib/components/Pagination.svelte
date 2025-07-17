<script lang="ts">
  export let currentPage: number = 1;
  export let totalPages: number = 1;
  export let totalItems: number = 0;
  export let itemsPerPage: number = 20;
  export let onPageChange: (page: number) => void = () => {};
  export let onItemsPerPageChange: (perPage: number) => void = () => {};

  $: startItem = (currentPage - 1) * itemsPerPage + 1;
  $: endItem = Math.min(currentPage * itemsPerPage, totalItems);

  // Generate page numbers to show
  $: visiblePages = (() => {
    const delta = 2; // Show 2 pages on each side of current page
    const range = [];
    const rangeWithDots = [];

    for (let i = Math.max(2, currentPage - delta); i <= Math.min(totalPages - 1, currentPage + delta); i++) {
      range.push(i);
    }

    if (currentPage - delta > 2) {
      rangeWithDots.push(1, '...');
    } else {
      rangeWithDots.push(1);
    }

    rangeWithDots.push(...range);

    if (currentPage + delta < totalPages - 1) {
      rangeWithDots.push('...', totalPages);
    } else {
      if (totalPages > 1) {
        rangeWithDots.push(totalPages);
      }
    }

    return rangeWithDots.filter((item, index, arr) => arr.indexOf(item) === index);
  })();

  function handlePageClick(page: number | string) {
    if (typeof page === 'number' && page !== currentPage) {
      onPageChange(page);
    }
  }

  function handlePrevious() {
    if (currentPage > 1) {
      onPageChange(currentPage - 1);
    }
  }

  function handleNext() {
    if (currentPage < totalPages) {
      onPageChange(currentPage + 1);
    }
  }

  function handleItemsPerPageChange(event: Event) {
    const target = event.target as HTMLSelectElement;
    const newPerPage = parseInt(target.value);
    onItemsPerPageChange(newPerPage);
  }
</script>

{#if totalPages > 1}
  <div class="flex flex-col sm:flex-row items-center justify-between space-y-4 sm:space-y-0 mt-6">
    <!-- Items info and per-page selector -->
    <div class="flex items-center space-x-4">
      <div class="text-sm text-gray-700">
        Showing {startItem} to {endItem} of {totalItems} games
      </div>
      
      <div class="flex items-center space-x-2">
        <label for="items-per-page" class="text-sm text-gray-700">
          Per page:
        </label>
        <select
          id="items-per-page"
          value={itemsPerPage}
          on:change={handleItemsPerPageChange}
          class="px-2 py-1 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option value="10">10</option>
          <option value="20">20</option>
          <option value="50">50</option>
          <option value="100">100</option>
        </select>
      </div>
    </div>

    <!-- Pagination controls -->
    <div class="flex items-center space-x-1">
      <!-- Previous button -->
      <button
        on:click={handlePrevious}
        disabled={currentPage <= 1}
        class="px-3 py-2 text-sm font-medium text-gray-500 bg-white border border-gray-300 rounded-l-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Previous
      </button>

      <!-- Page numbers -->
      {#each visiblePages as page}
        {#if page === '...'}
          <span class="px-3 py-2 text-sm text-gray-500">...</span>
        {:else}
          <button
            on:click={() => handlePageClick(page)}
            class="px-3 py-2 text-sm font-medium border border-gray-300 transition-colors {page === currentPage
              ? 'bg-blue-600 text-white border-blue-600'
              : 'text-gray-500 bg-white hover:bg-gray-50'}"
          >
            {page}
          </button>
        {/if}
      {/each}

      <!-- Next button -->
      <button
        on:click={handleNext}
        disabled={currentPage >= totalPages}
        class="px-3 py-2 text-sm font-medium text-gray-500 bg-white border border-gray-300 rounded-r-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Next
      </button>
    </div>
  </div>
{/if}