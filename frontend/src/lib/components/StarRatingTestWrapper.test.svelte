<script lang="ts">
  import StarRating from './StarRating.svelte';

  interface Props {
    initialValue?: number | null;
    readonly?: boolean;
    disabled?: boolean;
    clearable?: boolean;
    size?: 'sm' | 'md' | 'lg';
  }

  let {
    initialValue = null,
    readonly = false,
    disabled = false,
    clearable = true,
    size = 'md'
  }: Props = $props();

  let value = $state(initialValue);

  // Expose getValue method for testing
  export function getValue() {
    return value;
  }
</script>

<StarRating
  bind:value
  {readonly}
  {disabled}
  {clearable}
  {size}
/>