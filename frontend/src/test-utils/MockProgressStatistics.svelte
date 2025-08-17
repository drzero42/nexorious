<!-- Mock ProgressStatistics component for testing -->
<script lang="ts">
  // Accept props but render simple mock content in tests
  interface Props {
    userGames?: any[];
    className?: string;
  }
  
  let { userGames: _userGames = [], className = '' }: Props = $props();
</script>

<!-- Simple mock content for tests -->
<div class={className}>ProgressStatistics Mock</div>