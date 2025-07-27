<script lang="ts">
  import { createEventDispatcher } from 'svelte';

  interface Props {
    value: number;
    disabled?: boolean;
    class?: string;
    id?: string;
    name?: string;
    placeholder?: string;
  }

  let { 
    value = $bindable(), 
    disabled = false, 
    class: className = '', 
    id, 
    name, 
    placeholder = 'Enter time played...'
  }: Props = $props();

  const dispatch = createEventDispatcher<{
    change: { value: number };
  }>();

  let hours = $state(Math.floor(value));
  let minutes = $state(Math.floor((value - Math.floor(value)) * 60));
  let inputMode: 'simple' | 'detailed' = $state('simple');

  // Update the bound value when hours or minutes change
  function updateValue() {
    const newValue = hours + (minutes / 60);
    value = Math.round(newValue * 100) / 100; // Round to 2 decimal places
    dispatch('change', { value });
  }

  // Update internal state when external value changes
  $effect(() => {
    hours = Math.floor(value);
    minutes = Math.floor((value - hours) * 60);
  });

  function handleSimpleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    const newValue = parseFloat(target.value) || 0;
    value = newValue;
    dispatch('change', { value: newValue });
  }

  function incrementHours() {
    hours = Math.max(0, hours + 1);
    updateValue();
  }

  function decrementHours() {
    hours = Math.max(0, hours - 1);
    updateValue();
  }

  function incrementMinutes() {
    minutes = Math.min(59, minutes + 15); // Increment by 15 minutes
    updateValue();
  }

  function decrementMinutes() {
    minutes = Math.max(0, minutes - 15); // Decrement by 15 minutes
    updateValue();
  }

  function addQuickTime(timeValue: number) {
    value = Math.max(0, value + timeValue);
    dispatch('change', { value });
  }
</script>

<div class="space-y-3 {className}">
  <!-- Mode Toggle -->
  <div class="flex items-center space-x-2">
    <button
      type="button"
      onclick={() => inputMode = 'simple'}
      class="px-3 py-1 text-xs rounded-full transition-colors duration-200 {inputMode === 'simple' ? 'bg-blue-100 text-blue-800' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}"
      {disabled}
    >
      Simple
    </button>
    <button
      type="button"
      onclick={() => inputMode = 'detailed'}
      class="px-3 py-1 text-xs rounded-full transition-colors duration-200 {inputMode === 'detailed' ? 'bg-blue-100 text-blue-800' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}"
      {disabled}
    >
      Detailed
    </button>
  </div>

  {#if inputMode === 'simple'}
    <!-- Simple Number Input -->
    <div class="relative">
      <input
        {id}
        {name}
        {disabled}
        {placeholder}
        type="number"
        min="0"
        step="0.25"
        bind:value
        onchange={handleSimpleInput}
        class="form-input pr-12"
      />
      <div class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
        <span class="text-gray-500 text-sm">hours</span>
      </div>
    </div>
  {:else}
    <!-- Detailed Hours and Minutes Input -->
    <div class="grid grid-cols-2 gap-4">
      <!-- Hours -->
      <div class="space-y-2">
        <label for="hours-input" class="block text-sm font-medium text-gray-700">Hours</label>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={decrementHours}
            {disabled}
            class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-gray-100 text-gray-600 hover:bg-gray-200 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            aria-label="Decrease hours"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
            </svg>
          </button>
          <input
            id="hours-input"
            type="number"
            min="0"
            bind:value={hours}
            onchange={updateValue}
            {disabled}
            class="form-input text-center w-16"
          />
          <button
            type="button"
            onclick={incrementHours}
            {disabled}
            class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-gray-100 text-gray-600 hover:bg-gray-200 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            aria-label="Increase hours"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
            </svg>
          </button>
        </div>
      </div>

      <!-- Minutes -->
      <div class="space-y-2">
        <label for="minutes-input" class="block text-sm font-medium text-gray-700">Minutes</label>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={decrementMinutes}
            {disabled}
            class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-gray-100 text-gray-600 hover:bg-gray-200 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            aria-label="Decrease minutes"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
            </svg>
          </button>
          <input
            id="minutes-input"
            type="number"
            min="0"
            max="59"
            bind:value={minutes}
            onchange={updateValue}
            {disabled}
            class="form-input text-center w-16"
          />
          <button
            type="button"
            onclick={incrementMinutes}
            {disabled}
            class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-gray-100 text-gray-600 hover:bg-gray-200 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            aria-label="Increase minutes"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Quick Add Buttons -->
    <div class="pt-2">
      <div class="block text-sm font-medium text-gray-700 mb-2">Quick Add</div>
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          onclick={() => addQuickTime(0.5)}
          {disabled}
          class="px-3 py-1 text-xs bg-blue-50 text-blue-700 rounded-full hover:bg-blue-100 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          +30m
        </button>
        <button
          type="button"
          onclick={() => addQuickTime(1)}
          {disabled}
          class="px-3 py-1 text-xs bg-blue-50 text-blue-700 rounded-full hover:bg-blue-100 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          +1h
        </button>
        <button
          type="button"
          onclick={() => addQuickTime(2)}
          {disabled}
          class="px-3 py-1 text-xs bg-blue-50 text-blue-700 rounded-full hover:bg-blue-100 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          +2h
        </button>
        <button
          type="button"
          onclick={() => addQuickTime(5)}
          {disabled}
          class="px-3 py-1 text-xs bg-blue-50 text-blue-700 rounded-full hover:bg-blue-100 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          +5h
        </button>
      </div>
    </div>
  {/if}

  <!-- Current Value Display -->
  <div class="text-xs text-gray-500 text-center">
    Total: {value}h ({Math.floor(value)}h {Math.floor((value - Math.floor(value)) * 60)}m)
  </div>
</div>