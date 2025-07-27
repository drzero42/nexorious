import { browser } from '$app/environment';
import { auth } from '$lib/stores';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
  // Check if setup is actually needed
  if (browser) {
    const setupStatus = await auth.checkSetupStatus();
    
    if (!setupStatus.needs_setup) {
      // If setup is not needed, redirect to login
      throw redirect(302, '/login');
    }
  }

  return {};
};