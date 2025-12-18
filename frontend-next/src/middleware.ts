import { NextResponse, NextRequest } from 'next/server';

// For server-side requests (middleware), use internal Docker network URL
// INTERNAL_API_URL is for container-to-container communication
// Falls back to NEXT_PUBLIC_API_URL for non-Docker environments
const API_URL = process.env.INTERNAL_API_URL || process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000/api';

export async function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Skip if already on setup page
  if (pathname === '/setup') {
    return NextResponse.next();
  }

  try {
    // Check setup status from backend
    const response = await fetch(`${API_URL}/auth/setup/status`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
      },
      // Don't cache this response
      cache: 'no-store',
    });

    if (response.ok) {
      const data = await response.json();
      if (data.needs_setup) {
        // Redirect to setup page
        return NextResponse.redirect(new URL('/setup', request.url));
      }
    }
  } catch {
    // If backend is unavailable, continue without redirect
    // User will see an error when they try to login/use the app
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    // Match all paths except:
    // - API routes
    // - Static files
    // - Next.js internals
    '/((?!api|_next/static|_next/image|favicon.ico|.*\\..*).*)',
  ],
};
