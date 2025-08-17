<script lang="ts">
  interface Props {
    currentPage?: number;
    totalPages?: number;
    totalItems?: number;
    itemsPerPage?: number;
    onPageChange?: (page: number) => void;
    onItemsPerPageChange?: (perPage: number) => void;
  }
  
  let { 
    currentPage = 1,
    totalPages = 1,
    totalItems = 0,
    itemsPerPage = 20,
    onPageChange = () => {},
    onItemsPerPageChange = () => {}
  }: Props = $props();

  const startItem = $derived((currentPage - 1) * itemsPerPage + 1);
  const endItem = $derived(Math.min(currentPage * itemsPerPage, totalItems));

  // Generate page numbers to show
  const visiblePages: (number | string)[] = $derived((() => {
    const delta = 2; // Show 2 pages on each side of current page
    const range: number[] = [];
    const rangeWithDots: (number | string)[] = [];

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
  })());

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
          onchange={handleItemsPerPageChange}
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
        onclick={handlePrevious}
        disabled={currentPage <= 1}
        class="btn-secondary text-sm px-3 py-2 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Previous
      </button>

      <!-- Page numbers -->
      {#each visiblePages as page}
        {#if page === '...'}
          <span>...</span>
        {:else}
          <button
            onclick={() => handlePageClick(page)}
            class="px-3 py-2 text-sm font-medium rounded-md transition-colors duration-200 min-w-[40px] {page === currentPage ? 'btn-primary' : 'btn-secondary'}"
          >
            {page}
          </button>
        {/if}
      {/each}

      <!-- Next button -->
      <button
        onclick={handleNext}
        disabled={currentPage >= totalPages}
        class="btn-secondary text-sm px-3 py-2 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Next
      </button>
    </div>
  </div>
{/if}