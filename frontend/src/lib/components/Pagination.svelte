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
  <div>
    <!-- Items info and per-page selector -->
    <div>
      <div>
        Showing {startItem} to {endItem} of {totalItems} games
      </div>
      
      <div>
        <label for="items-per-page">
          Per page:
        </label>
        <select
          id="items-per-page"
          value={itemsPerPage}
          on:change={handleItemsPerPageChange}
        >
          <option value="10">10</option>
          <option value="20">20</option>
          <option value="50">50</option>
          <option value="100">100</option>
        </select>
      </div>
    </div>

    <!-- Pagination controls -->
    <div>
      <!-- Previous button -->
      <button
        on:click={handlePrevious}
        disabled={currentPage <= 1}
      >
        Previous
      </button>

      <!-- Page numbers -->
      {#each visiblePages as page}
        {#if page === '...'}
          <span>...</span>
        {:else}
          <button
            on:click={() => handlePageClick(page)}
          >
            {page}
          </button>
        {/if}
      {/each}

      <!-- Next button -->
      <button
        on:click={handleNext}
        disabled={currentPage >= totalPages}
      >
        Next
      </button>
    </div>
  </div>
{/if}