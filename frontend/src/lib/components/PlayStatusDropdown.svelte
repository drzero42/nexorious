<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { PlayStatus } from '$lib/stores/user-games.svelte';

  interface Props {
    value: PlayStatus;
    disabled?: boolean;
    class?: string;
    id?: string;
    name?: string;
    onchange?: (event: CustomEvent<{ value: PlayStatus }>) => void;
  }

  let { value = $bindable(), disabled = false, class: className = '', id, name, onchange }: Props = $props();

  const dispatch = createEventDispatcher<{
    change: { value: PlayStatus };
  }>();

  const statusOptions = [
    {
      value: 'not_started' as PlayStatus,
      label: 'Not Started',
      description: "Haven't begun playing",
      color: 'text-gray-600 bg-gray-100 border-gray-200'
    },
    {
      value: 'in_progress' as PlayStatus,
      label: 'In Progress',
      description: 'Currently playing',
      color: 'text-blue-600 bg-blue-100 border-blue-200'
    },
    {
      value: 'completed' as PlayStatus,
      label: 'Completed',
      description: 'Finished main story/campaign',
      color: 'text-green-600 bg-green-100 border-green-200'
    },
    {
      value: 'mastered' as PlayStatus,
      label: 'Mastered',
      description: 'Completed main story plus all side quests and content',
      color: 'text-purple-600 bg-purple-100 border-purple-200'
    },
    {
      value: 'dominated' as PlayStatus,
      label: 'Dominated',
      description: '100% completion including all achievements/trophies',
      color: 'text-yellow-600 bg-yellow-100 border-yellow-200'
    },
    {
      value: 'shelved' as PlayStatus,
      label: 'Shelved',
      description: 'Temporarily paused with intent to return',
      color: 'text-orange-600 bg-orange-100 border-orange-200'
    },
    {
      value: 'dropped' as PlayStatus,
      label: 'Dropped',
      description: 'Permanently abandoned',
      color: 'text-red-600 bg-red-100 border-red-200'
    },
    {
      value: 'replay' as PlayStatus,
      label: 'Replay',
      description: 'Playing again after previous completion',
      color: 'text-indigo-600 bg-indigo-100 border-indigo-200'
    }
  ];

  function handleChange(event: Event) {
    const target = event.target as HTMLSelectElement;
    const newValue = target.value as PlayStatus;
    value = newValue;
    dispatch('change', { value: newValue });
    onchange?.(new CustomEvent('change', { detail: { value: newValue } }));
  }

  function getStatusColor(statusValue: PlayStatus): string {
    const option = statusOptions.find(opt => opt.value === statusValue);
    return option?.color || 'text-gray-600 bg-gray-100 border-gray-200';
  }

  const selectedOption = $derived(statusOptions.find(opt => opt.value === value));
</script>

<div class="relative {className}">
  <select
    {id}
    {name}
    {disabled}
    bind:value
    onchange={handleChange}
    class="form-input pr-10 appearance-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 transition-colors duration-200"
    title={selectedOption?.description}
  >
    {#each statusOptions as option}
      <option value={option.value}>
        {option.label}
      </option>
    {/each}
  </select>
  
  <!-- Custom dropdown arrow -->
  <div class="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none">
    <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
    </svg>
  </div>
  
  <!-- Status indicator badge -->
  <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
    <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium {getStatusColor(value)}">
      {selectedOption?.label}
    </span>
  </div>
</div>

<!-- Tooltip/Help text -->
{#if selectedOption}
  <p class="mt-1 text-xs text-gray-500">
    {selectedOption.description}
  </p>
{/if}

<style>
  select {
    padding-left: 120px; /* Make room for the status badge */
  }
  
  /* Hide default select arrow in some browsers */
  select::-ms-expand {
    display: none;
  }
</style>