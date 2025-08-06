import { browser } from '$app/environment';
import { auth } from '$lib/stores';
import { redirect } from '@sveltejs/kit';
import type { LayoutLoad } from './$types';

export const ssr = false; // Disable SSR for this app

export const load: LayoutLoad = async ({ url, fetch }) => {
  // Only check setup status in the browser and not on the setup page itself
  if (browser && url.pathname !== '/setup') {
    try {
      const setupStatus = await auth.checkSetupStatus(fetch);
      
      if (setupStatus.needs_setup) {
        // Redirect to setup page if initial admin setup is needed
        throw redirect(302, '/setup');
      }
    } catch (error) {
      // If it's a redirect, re-throw it
      if (error && typeof error === 'object' && 'status' in error && 'location' in error) {
        throw error;
      }
      // Otherwise, log the error and continue
      console.error('Failed to check setup status:', error);
    }
  }

  return {};
};