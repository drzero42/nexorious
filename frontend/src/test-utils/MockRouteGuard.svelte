<!-- Mock RouteGuard component for testing that always renders authorized content -->
<script lang="ts">
  // Accept all props but ignore auth logic in tests
  export const redirectTo: string = '/login';
  export const requireAuth: boolean = true;
  export const requireAdmin: boolean = false;
</script>

<!-- In tests, always render the slot content without auth checks -->
<slot />