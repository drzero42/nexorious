import { http, HttpResponse } from "msw";
import type { User, LoginResponse, SetupStatusResponse } from "@/types";

// Match the API URL format from env.ts (development mode)
const API_URL = "http://localhost:8000/api";

// Mock data
export const mockUser: User = {
  id: "test-user-id",
  username: "testuser",
  isAdmin: false,
};

export const mockAdminUser: User = {
  id: "admin-user-id",
  username: "admin",
  isAdmin: true,
};

export const mockTokens = {
  access_token: "mock-access-token",
  refresh_token: "mock-refresh-token",
  token_type: "bearer",
  expires_in: 3600,
};

// Default handlers
export const handlers = [
  // Auth endpoints
  http.post(`${API_URL}/auth/login`, async ({ request }) => {
    const body = (await request.formData()) as FormData;
    const username = body.get("username");
    const password = body.get("password");

    if (username === "testuser" && password === "password123") {
      return HttpResponse.json<LoginResponse>(mockTokens);
    }

    if (username === "admin" && password === "admin123") {
      return HttpResponse.json<LoginResponse>(mockTokens);
    }

    return HttpResponse.json(
      { detail: "Invalid credentials" },
      { status: 401 }
    );
  }),

  http.get(`${API_URL}/auth/me`, ({ request }) => {
    const authHeader = request.headers.get("Authorization");

    if (!authHeader || !authHeader.startsWith("Bearer ")) {
      return HttpResponse.json({ detail: "Not authenticated" }, { status: 401 });
    }

    // Return different users based on token for testing
    const token = authHeader.replace("Bearer ", "");
    if (token === "admin-token") {
      return HttpResponse.json(mockAdminUser);
    }

    return HttpResponse.json(mockUser);
  }),

  http.post(`${API_URL}/auth/refresh`, async ({ request }) => {
    const body = (await request.json()) as { refresh_token: string };

    if (body.refresh_token === "mock-refresh-token") {
      return HttpResponse.json<LoginResponse>({
        ...mockTokens,
        access_token: "new-mock-access-token",
      });
    }

    return HttpResponse.json({ detail: "Invalid refresh token" }, { status: 401 });
  }),

  http.get(`${API_URL}/auth/setup/status`, () => {
    return HttpResponse.json<SetupStatusResponse>({ needs_setup: false });
  }),

  http.post(`${API_URL}/auth/setup/admin`, async ({ request }) => {
    const body = (await request.json()) as { username: string; password: string };

    if (body.username && body.password) {
      return HttpResponse.json(mockAdminUser);
    }

    return HttpResponse.json(
      { detail: "Invalid setup data" },
      { status: 400 }
    );
  }),

  // Platform endpoints
  http.get(`${API_URL}/platforms/`, () => {
    return HttpResponse.json([
      { id: 1, name: "PC", slug: "pc" },
      { id: 2, name: "PlayStation 5", slug: "ps5" },
      { id: 3, name: "Xbox Series X", slug: "xbox-series-x" },
    ]);
  }),

  // Game endpoints
  http.get(`${API_URL}/user-games/`, () => {
    return HttpResponse.json([]);
  }),

  http.get(`${API_URL}/user-games/:id`, ({ params }) => {
    const { id } = params;
    return HttpResponse.json({
      id,
      title: "Test Game",
      igdb_id: 12345,
      status: "backlog",
    });
  }),

  // Tags endpoints
  http.get(`${API_URL}/tags/`, () => {
    return HttpResponse.json([
      { id: 1, name: "RPG", color: "#FF5733" },
      { id: 2, name: "Action", color: "#33FF57" },
    ]);
  }),

  // IGDB endpoints
  http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get("query");

    if (!query) {
      return HttpResponse.json({ detail: "Query required" }, { status: 400 });
    }

    return HttpResponse.json([
      {
        id: 1,
        name: `${query} Game`,
        cover_url: "https://example.com/cover.jpg",
        release_date: "2024-01-01",
        platforms: ["PC", "PS5"],
      },
    ]);
  }),
];
