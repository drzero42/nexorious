# Svelte 5 Syntax Guide

Quick reference for writing optimal Svelte 5 code with runes and modern patterns.

## Core Runes Migration

```javascript
// Svelte 4 → Svelte 5
let count = 0;              → let count = $state(0);
$: doubled = count * 2;     → let doubled = $derived(count * 2);
$: console.log(count);      → $effect(() => console.log(count));
export let name;            → let { name } = $props();
```

## State Management

### Basic State
```javascript
// Reactive primitive
let count = $state(0);
count++; // Triggers reactivity

// Reactive objects (deep reactivity)
let user = $state({
  name: 'John',
  settings: { theme: 'dark' }
});
user.settings.theme = 'light'; // Triggers reactivity
```

### Derived State
```javascript
let count = $state(0);
let doubled = $derived(count * 2);
let isEven = $derived(count % 2 === 0);
```

### Effects
```javascript
let count = $state(0);

// Run on count changes
$effect(() => {
  console.log('Count is:', count);
});

// Cleanup function
$effect(() => {
  const interval = setInterval(() => count++, 1000);
  return () => clearInterval(interval);
});
```

## Props and Binding

### Props
```javascript
// Basic props
let { title, description = 'Default text' } = $props();

// Rest props
let { class: className, ...restProps } = $props();

// Bindable props
let { value = $bindable() } = $props();
```

### Event Handling
```svelte
<!-- Svelte 4 → Svelte 5 -->
<button on:click={handler}>    → <button onclick={handler}>
<input on:input={handler}>     → <input oninput={handler}>

<!-- Component events via callback props -->
<Child on:message={handler}>   → <Child onmessage={handler}>
```

## Component Patterns

### Modern Component Structure
```svelte
<script>
  // Props
  let { items, onSelect } = $props();
  
  // State
  let selected = $state(null);
  let filteredItems = $derived(
    items.filter(item => item.visible)
  );
  
  // Effects
  $effect(() => {
    if (selected) {
      onSelect?.(selected);
    }
  });
  
  function handleClick(item) {
    selected = item;
  }
</script>

<div>
  {#each filteredItems as item}
    <button onclick={() => handleClick(item)}>
      {item.name}
    </button>
  {/each}
</div>
```

### Shared State (.svelte.js/.svelte.ts)
```javascript
// shared-state.svelte.js
export const globalCounter = $state({ count: 0 });

export function increment() {
  globalCounter.count++;
}

// In components
import { globalCounter, increment } from './shared-state.svelte.js';
```

## Advanced Patterns

### Class Components
```javascript
class Counter {
  count = $state(0);
  doubled = $derived(this.count * 2);
  
  increment = () => {
    this.count++;
  }
}

let counter = new Counter();
```

### Raw State (Non-reactive)
```javascript
let rawData = $state.raw({
  largeArray: [/* thousands of items */]
});
```

### State Snapshots
```javascript
let todos = $state([]);
let snapshot = $state.snapshot(todos); // Static copy
```

## Migration Tips

### Event Handler Updates
```svelte
<!-- Multiple handlers need wrapper -->
<button 
  onclick={() => {
    handler1();
    handler2();
  }}
>
  Click me
</button>
```

### Component Instantiation
```javascript
// Svelte 4
new Component({ target, props });

// Svelte 5  
import { mount } from 'svelte';
mount(Component, { target, props });
```

### Store Migration
```javascript
// Svelte 4 store
import { writable } from 'svelte/store';
const count = writable(0);

// Svelte 5 shared state
// shared.svelte.js
export const count = $state({ value: 0 });
```

## Best Practices

1. **Use runes for all reactivity** - More explicit and consistent
2. **Shared state in .svelte.js files** - Universal reactivity
3. **Prefer $derived over manual calculations** - Automatic dependency tracking  
4. **Use $effect for side effects only** - Not for derived computations
5. **Destructure props immediately** - Clear component interface
6. **Use $bindable sparingly** - Only when true two-way binding needed

## Common Patterns

### Form Handling
```javascript
let { initialValue = '' } = $props();
let value = $state(initialValue);
let isDirty = $derived(value !== initialValue);
```

### Loading States
```javascript
let isLoading = $state(false);
let data = $state(null);

async function fetchData() {
  isLoading = true;
  try {
    data = await api.getData();
  } finally {
    isLoading = false;
  }
}
```

### Conditional Rendering
```svelte
{#if isLoading}
  <LoadingSpinner />
{:else if data}
  <DataList {data} />
{:else}
  <EmptyState />
{/if}
```