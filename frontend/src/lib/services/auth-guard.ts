import { auth } from '$lib/stores/auth.svelte';
import { browser } from '$app/environment';

export interface AuthCheckOptions {
  requireAuth?: boolean;
  requireAdmin?: boolean;
  skipRefresh?: boolean;
}

export interface AuthCheckResult {
  isAuthorized: boolean;
  shouldRedirect: boolean;
  redirectTo: string | null;
  reason?: string;
}

/**
 * Service for handling authentication checks and authorization logic
 * Separated from navigation concerns for better testability and reusability
 */
export class AuthGuardService {
  private static instance: AuthGuardService;

  static getInstance(): AuthGuardService {
    if (!AuthGuardService.instance) {
      AuthGuardService.instance = new AuthGuardService();
    }
    return AuthGuardService.instance;
  }

  /**
   * Perform authentication check with token refresh if needed
   */
  async checkAuthentication(options: AuthCheckOptions = {}): Promise<AuthCheckResult> {
    const {
      requireAuth = true,
      requireAdmin = false,
      skipRefresh = false
    } = options;

    if (!browser) {
      return { isAuthorized: false, shouldRedirect: false, redirectTo: null };
    }

    let authState = auth.value;

    // If we have tokens but no user, try to refresh (unless skipped)
    if (!skipRefresh && authState.accessToken && authState.refreshToken && !authState.user) {
      try {
        await auth.refreshAuth();
        authState = auth.value; // Re-read auth state after refresh
      } catch (error) {
        console.error('Failed to refresh auth:', error);
        // Continue with the check - might still be valid for public routes
      }
    }

    return this.evaluateAuthState(authState, { requireAuth, requireAdmin });
  }

  /**
   * Evaluate auth state against requirements without side effects
   */
  evaluateAuthState(authState: any, options: AuthCheckOptions): AuthCheckResult {
    const { requireAuth = true, requireAdmin = false } = options;

    // Check authentication requirements
    if (requireAuth && !authState.user) {
      return {
        isAuthorized: false,
        shouldRedirect: true,
        redirectTo: '/login',
        reason: 'Authentication required'
      };
    }

    // Check admin requirements
    if (requireAdmin && (!authState.user || !authState.user.isAdmin)) {
      return {
        isAuthorized: false,
        shouldRedirect: true,
        redirectTo: '/',
        reason: 'Admin privileges required'
      };
    }

    // Handle logged-in user on public pages (like login/register)
    if (!requireAuth && authState.user) {
      return {
        isAuthorized: false,
        shouldRedirect: true,
        redirectTo: '/games',
        reason: 'Already authenticated'
      };
    }

    // User is authorized
    return {
      isAuthorized: true,
      shouldRedirect: false,
      redirectTo: null
    };
  }

  /**
   * Check if current user has admin privileges
   */
  isAdmin(): boolean {
    const authState = auth.value;
    return !!(authState.user && authState.user.isAdmin);
  }

  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    const authState = auth.value;
    return !!authState.user;
  }

  /**
   * Get current user info
   */
  getCurrentUser() {
    return auth.value.user;
  }
}

// Export singleton instance
export const authGuard = AuthGuardService.getInstance();