<script lang="ts">
  interface Props {
    value: number;
    max?: number;
    label?: string;
    showPercentage?: boolean;
    color?: 'blue' | 'green' | 'purple' | 'yellow' | 'orange' | 'red' | 'gray';
    size?: 'sm' | 'md' | 'lg';
    animated?: boolean;
    class?: string;
  }

  let {
    value = $bindable(),
    max = 100,
    label = '',
    showPercentage = true,
    color = 'blue',
    size = 'md',
    animated = true,
    class: className = ''
  }: Props = $props();

  const percentage = $derived(Math.min((value / max) * 100, 100));
  const formattedPercentage = $derived(percentage.toFixed(1));

  const colorClasses = {
    blue: 'bg-blue-500',
    green: 'bg-green-500',
    purple: 'bg-purple-500',
    yellow: 'bg-yellow-500',
    orange: 'bg-orange-500',
    red: 'bg-red-500',
    gray: 'bg-gray-500'
  };

  const sizeClasses = {
    sm: 'h-2',
    md: 'h-3',
    lg: 'h-4'
  };

  const bgColorClasses = {
    blue: 'bg-blue-100',
    green: 'bg-green-100',
    purple: 'bg-purple-100',
    yellow: 'bg-yellow-100',
    orange: 'bg-orange-100',
    red: 'bg-red-100',
    gray: 'bg-gray-200'
  };
</script>

<div class="w-full {className}">
  {#if label || showPercentage}
    <div class="flex justify-between items-center mb-1">
      {#if label}
        <span class="text-sm font-medium text-gray-700">{label}</span>
      {/if}
      {#if showPercentage}
        <span class="text-sm text-gray-600">{formattedPercentage}%</span>
      {/if}
    </div>
  {/if}
  
  <div 
    class="w-full {bgColorClasses[color]} rounded-full overflow-hidden {sizeClasses[size]}"
    role="progressbar"
    aria-valuenow={value}
    aria-valuemin={0}
    aria-valuemax={max}
    aria-label={label || 'Progress'}
  >
    <div 
      class="{colorClasses[color]} {sizeClasses[size]} rounded-full {animated ? 'transition-all duration-300 ease-out' : ''}"
      style="width: {percentage}%"
    ></div>
  </div>
</div>

<style>
  /* Add a subtle shine effect for better visual appeal */
  div[role="progressbar"] > div {
    position: relative;
    overflow: hidden;
  }
  
  div[role="progressbar"] > div::after {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    bottom: 0;
    right: 0;
    background: linear-gradient(
      90deg,
      transparent,
      rgba(255, 255, 255, 0.2),
      transparent
    );
    transform: translateX(-100%);
  }
  
  div[role="progressbar"] > div:not(.transition-none)::after {
    animation: shimmer 2s infinite;
  }
  
  @keyframes shimmer {
    100% {
      transform: translateX(100%);
    }
  }
</style>