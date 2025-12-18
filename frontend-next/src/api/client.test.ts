import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/mocks/server";
import {
  ApiErrorException,
  setAuthHandlers,
  apiCall,
  api,
} from "./client";

// In test environment, NODE_ENV is 'test' so apiUrl defaults to '/api'
// MSW intercepts relative URLs with the origin prepended
const API_URL = "/api";

describe("client.ts", () => {
  // Auth handler mocks with proper typing
  let mockGetAccessToken: Mock<() => string | null>;
  let mockRefreshTokens: Mock<() => Promise<boolean>>;
  let mockLogout: Mock<() => void>;

  beforeEach(() => {
    vi.clearAllMocks();

    // Reset auth handlers with fresh mocks
    mockGetAccessToken = vi.fn<() => string | null>().mockReturnValue("test-access-token");
    mockRefreshTokens = vi.fn<() => Promise<boolean>>().mockResolvedValue(false);
    mockLogout = vi.fn<() => void>();

    setAuthHandlers(mockGetAccessToken, mockRefreshTokens, mockLogout);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("ApiErrorException", () => {
    it("creates error with message and status", () => {
      const error = new ApiErrorException("Not found", 404);

      expect(error).toBeInstanceOf(Error);
      expect(error.message).toBe("Not found");
      expect(error.status).toBe(404);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe("ApiErrorException");
    });

    it("creates error with details", () => {
      const details = { field: "email", reason: "invalid" };
      const error = new ApiErrorException("Validation failed", 400, details);

      expect(error.message).toBe("Validation failed");
      expect(error.status).toBe(400);
      expect(error.details).toEqual(details);
    });

    it("is throwable and catchable", () => {
      expect(() => {
        throw new ApiErrorException("Test error", 500);
      }).toThrow(ApiErrorException);
    });
  });

  describe("setAuthHandlers", () => {
    it("sets custom auth handlers", async () => {
      const customGetter = vi.fn().mockReturnValue("custom-token");
      const customRefresher = vi.fn().mockResolvedValue(true);
      const customLogout = vi.fn();

      setAuthHandlers(customGetter, customRefresher, customLogout);

      server.use(
        http.get(`${API_URL}/test-endpoint`, ({ request }) => {
          const auth = request.headers.get("Authorization");
          return HttpResponse.json({ auth });
        })
      );

      await apiCall("/test-endpoint");

      expect(customGetter).toHaveBeenCalled();
    });
  });

  describe("apiCall", () => {
    describe("URL construction", () => {
      it("constructs URL with path starting with slash", async () => {
        server.use(
          http.get(`${API_URL}/test-path`, () => {
            return HttpResponse.json({ success: true });
          })
        );

        const response = await apiCall("/test-path");
        const data = await response.json();

        expect(data.success).toBe(true);
      });

      it("constructs URL with path not starting with slash", async () => {
        server.use(
          http.get(`${API_URL}/test-path`, () => {
            return HttpResponse.json({ success: true });
          })
        );

        const response = await apiCall("test-path");
        const data = await response.json();

        expect(data.success).toBe(true);
      });

      it("appends query params to URL", async () => {
        server.use(
          http.get(`${API_URL}/search`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              query: url.searchParams.get("query"),
              limit: url.searchParams.get("limit"),
            });
          })
        );

        const response = await apiCall("/search", {
          params: { query: "test", limit: 10 },
        });
        const data = await response.json();

        expect(data.query).toBe("test");
        expect(data.limit).toBe("10");
      });

      it("excludes undefined params", async () => {
        server.use(
          http.get(`${API_URL}/search`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              hasQuery: url.searchParams.has("query"),
              hasUndefined: url.searchParams.has("undefined_param"),
            });
          })
        );

        const response = await apiCall("/search", {
          params: { query: "test", undefined_param: undefined },
        });
        const data = await response.json();

        expect(data.hasQuery).toBe(true);
        expect(data.hasUndefined).toBe(false);
      });

      it("handles boolean params", async () => {
        server.use(
          http.get(`${API_URL}/items`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              active: url.searchParams.get("active"),
            });
          })
        );

        const response = await apiCall("/items", {
          params: { active: true },
        });
        const data = await response.json();

        expect(data.active).toBe("true");
      });
    });

    describe("headers", () => {
      it("sets Content-Type to application/json by default", async () => {
        server.use(
          http.get(`${API_URL}/test`, ({ request }) => {
            return HttpResponse.json({
              contentType: request.headers.get("Content-Type"),
            });
          })
        );

        const response = await apiCall("/test");
        const data = await response.json();

        expect(data.contentType).toBe("application/json");
      });

      it("allows custom headers", async () => {
        server.use(
          http.get(`${API_URL}/test`, ({ request }) => {
            return HttpResponse.json({
              customHeader: request.headers.get("X-Custom-Header"),
            });
          })
        );

        const response = await apiCall("/test", {
          headers: { "X-Custom-Header": "custom-value" },
        });
        const data = await response.json();

        expect(data.customHeader).toBe("custom-value");
      });

      it("adds Authorization header when authenticated", async () => {
        server.use(
          http.get(`${API_URL}/protected`, ({ request }) => {
            return HttpResponse.json({
              auth: request.headers.get("Authorization"),
            });
          })
        );

        const response = await apiCall("/protected");
        const data = await response.json();

        expect(data.auth).toBe("Bearer test-access-token");
      });
    });

    describe("authentication", () => {
      it("throws error when not authenticated and skipAuth is false", async () => {
        mockGetAccessToken.mockReturnValue(null);

        await expect(apiCall("/protected")).rejects.toThrow(ApiErrorException);
        await expect(apiCall("/protected")).rejects.toMatchObject({
          message: "Not authenticated",
          status: 401,
        });
      });

      it("skips auth header when skipAuth is true", async () => {
        mockGetAccessToken.mockReturnValue(null);

        server.use(
          http.get(`${API_URL}/public`, ({ request }) => {
            return HttpResponse.json({
              auth: request.headers.get("Authorization"),
            });
          })
        );

        const response = await apiCall("/public", { skipAuth: true });
        const data = await response.json();

        expect(data.auth).toBeNull();
      });

      it("makes request without checking token when skipAuth is true", async () => {
        mockGetAccessToken.mockReturnValue(null);

        server.use(
          http.get(`${API_URL}/public`, () => {
            return HttpResponse.json({ success: true });
          })
        );

        const response = await apiCall("/public", { skipAuth: true });
        const data = await response.json();

        expect(data.success).toBe(true);
        // getAccessToken should not be called when skipAuth is true
        expect(mockGetAccessToken).not.toHaveBeenCalled();
      });
    });

    describe("token refresh on 401", () => {
      it("attempts token refresh on 401 response", async () => {
        let requestCount = 0;
        mockRefreshTokens.mockResolvedValue(true);
        mockGetAccessToken
          .mockReturnValueOnce("old-token")
          .mockReturnValueOnce("new-token");

        server.use(
          http.get(`${API_URL}/protected`, ({ request }) => {
            requestCount++;
            const auth = request.headers.get("Authorization");

            if (auth === "Bearer old-token") {
              return HttpResponse.json(
                { detail: "Token expired" },
                { status: 401 }
              );
            }

            if (auth === "Bearer new-token") {
              return HttpResponse.json({ success: true });
            }

            return HttpResponse.json({ detail: "Unknown token" }, { status: 401 });
          })
        );

        const response = await apiCall("/protected");
        const data = await response.json();

        expect(mockRefreshTokens).toHaveBeenCalledTimes(1);
        expect(requestCount).toBe(2);
        expect(data.success).toBe(true);
      });

      it("calls logout when refresh fails", async () => {
        mockRefreshTokens.mockResolvedValue(false);

        server.use(
          http.get(`${API_URL}/protected`, () => {
            return HttpResponse.json(
              { detail: "Token expired" },
              { status: 401 }
            );
          })
        );

        await expect(apiCall("/protected")).rejects.toThrow(ApiErrorException);

        expect(mockRefreshTokens).toHaveBeenCalledTimes(1);
        expect(mockLogout).toHaveBeenCalledTimes(1);
      });

      it("does not attempt refresh when skipAuth is true", async () => {
        server.use(
          http.get(`${API_URL}/public`, () => {
            return HttpResponse.json({ detail: "Unauthorized" }, { status: 401 });
          })
        );

        await expect(apiCall("/public", { skipAuth: true })).rejects.toThrow(
          ApiErrorException
        );

        expect(mockRefreshTokens).not.toHaveBeenCalled();
        expect(mockLogout).not.toHaveBeenCalled();
      });

      it("deduplicates concurrent refresh requests", async () => {
        let refreshCallCount = 0;
        mockRefreshTokens.mockImplementation(async () => {
          refreshCallCount++;
          await new Promise((resolve) => setTimeout(resolve, 50));
          return true;
        });

        mockGetAccessToken
          .mockReturnValueOnce("old-token")
          .mockReturnValueOnce("old-token")
          .mockReturnValue("new-token");

        server.use(
          http.get(`${API_URL}/protected`, ({ request }) => {
            const auth = request.headers.get("Authorization");
            if (auth === "Bearer old-token") {
              return HttpResponse.json({ detail: "Expired" }, { status: 401 });
            }
            return HttpResponse.json({ success: true });
          })
        );

        // Make concurrent requests that both trigger refresh
        const [response1, response2] = await Promise.all([
          apiCall("/protected"),
          apiCall("/protected"),
        ]);

        const data1 = await response1.json();
        const data2 = await response2.json();

        // Both requests should succeed
        expect(data1.success).toBe(true);
        expect(data2.success).toBe(true);

        // But refresh should only be called once due to deduplication
        expect(refreshCallCount).toBe(1);
      });
    });

    describe("error handling", () => {
      it("throws ApiErrorException on non-ok response", async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json(
              { detail: "Something went wrong" },
              { status: 500 }
            );
          })
        );

        await expect(apiCall("/error")).rejects.toThrow(ApiErrorException);
        await expect(apiCall("/error")).rejects.toMatchObject({
          message: "Something went wrong",
          status: 500,
        });
      });

      it("extracts error message from detail field", async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json(
              { detail: "Detailed error message" },
              { status: 400 }
            );
          })
        );

        await expect(apiCall("/error")).rejects.toMatchObject({
          message: "Detailed error message",
        });
      });

      it("extracts error message from message field", async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json(
              { message: "Error message field" },
              { status: 400 }
            );
          })
        );

        await expect(apiCall("/error")).rejects.toMatchObject({
          message: "Error message field",
        });
      });

      it("uses default error message when response is not JSON", async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return new HttpResponse("Server Error", {
              status: 500,
              statusText: "Internal Server Error",
            });
          })
        );

        await expect(apiCall("/error")).rejects.toMatchObject({
          message: "HTTP 500: Internal Server Error",
          status: 500,
        });
      });

      it("preserves error details in exception", async () => {
        const errorDetails = {
          detail: "Validation failed",
          errors: [
            { field: "email", message: "Invalid email" },
            { field: "password", message: "Too short" },
          ],
        };

        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json(errorDetails, { status: 422 });
          })
        );

        try {
          await apiCall("/error");
          expect.fail("Should have thrown");
        } catch (error) {
          expect(error).toBeInstanceOf(ApiErrorException);
          const apiError = error as ApiErrorException;
          expect(apiError.details).toEqual(errorDetails);
        }
      });
    });

    describe("HTTP methods", () => {
      it("supports GET method", async () => {
        server.use(
          http.get(`${API_URL}/resource`, () => {
            return HttpResponse.json({ method: "GET" });
          })
        );

        const response = await apiCall("/resource", { method: "GET" });
        const data = await response.json();

        expect(data.method).toBe("GET");
      });

      it("supports POST method with body", async () => {
        server.use(
          http.post(`${API_URL}/resource`, async ({ request }) => {
            const body = await request.json();
            return HttpResponse.json({ method: "POST", body });
          })
        );

        const response = await apiCall("/resource", {
          method: "POST",
          body: JSON.stringify({ name: "test" }),
        });
        const data = await response.json();

        expect(data.method).toBe("POST");
        expect(data.body).toEqual({ name: "test" });
      });

      it("supports PUT method", async () => {
        server.use(
          http.put(`${API_URL}/resource/1`, async ({ request }) => {
            const body = await request.json();
            return HttpResponse.json({ method: "PUT", body });
          })
        );

        const response = await apiCall("/resource/1", {
          method: "PUT",
          body: JSON.stringify({ name: "updated" }),
        });
        const data = await response.json();

        expect(data.method).toBe("PUT");
        expect(data.body).toEqual({ name: "updated" });
      });

      it("supports DELETE method", async () => {
        server.use(
          http.delete(`${API_URL}/resource/1`, () => {
            return HttpResponse.json({ method: "DELETE" });
          })
        );

        const response = await apiCall("/resource/1", { method: "DELETE" });
        const data = await response.json();

        expect(data.method).toBe("DELETE");
      });
    });
  });

  describe("api helper object", () => {
    describe("api.get", () => {
      it("makes GET request and returns parsed JSON", async () => {
        server.use(
          http.get(`${API_URL}/items`, () => {
            return HttpResponse.json({ items: [1, 2, 3] });
          })
        );

        const data = await api.get<{ items: number[] }>("/items");

        expect(data.items).toEqual([1, 2, 3]);
      });

      it("passes options to apiCall", async () => {
        server.use(
          http.get(`${API_URL}/items`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              page: url.searchParams.get("page"),
            });
          })
        );

        const data = await api.get<{ page: string }>("/items", {
          params: { page: 2 },
        });

        expect(data.page).toBe("2");
      });
    });

    describe("api.post", () => {
      it("makes POST request with body and returns parsed JSON", async () => {
        server.use(
          http.post(`${API_URL}/items`, async ({ request }) => {
            const body = (await request.json()) as { name: string };
            return HttpResponse.json({ id: 1, name: body.name });
          })
        );

        const data = await api.post<{ id: number; name: string }>("/items", {
          name: "New Item",
        });

        expect(data).toEqual({ id: 1, name: "New Item" });
      });

      it("makes POST request without body", async () => {
        server.use(
          http.post(`${API_URL}/action`, () => {
            return HttpResponse.json({ success: true });
          })
        );

        const data = await api.post<{ success: boolean }>("/action");

        expect(data.success).toBe(true);
      });

      it("passes options to apiCall", async () => {
        mockGetAccessToken.mockReturnValue(null);

        server.use(
          http.post(`${API_URL}/public`, () => {
            return HttpResponse.json({ public: true });
          })
        );

        const data = await api.post<{ public: boolean }>(
          "/public",
          undefined,
          { skipAuth: true }
        );

        expect(data.public).toBe(true);
      });
    });

    describe("api.put", () => {
      it("makes PUT request with body and returns parsed JSON", async () => {
        server.use(
          http.put(`${API_URL}/items/1`, async ({ request }) => {
            const body = (await request.json()) as { name: string };
            return HttpResponse.json({ id: 1, name: body.name });
          })
        );

        const data = await api.put<{ id: number; name: string }>("/items/1", {
          name: "Updated Item",
        });

        expect(data).toEqual({ id: 1, name: "Updated Item" });
      });

      it("makes PUT request without body", async () => {
        server.use(
          http.put(`${API_URL}/items/1/activate`, () => {
            return HttpResponse.json({ activated: true });
          })
        );

        const data = await api.put<{ activated: boolean }>("/items/1/activate");

        expect(data.activated).toBe(true);
      });
    });

    describe("api.patch", () => {
      it("makes PATCH request with partial body and returns parsed JSON", async () => {
        server.use(
          http.patch(`${API_URL}/items/1`, async ({ request }) => {
            const body = (await request.json()) as { status: string };
            return HttpResponse.json({ id: 1, name: "Original", status: body.status });
          })
        );

        const data = await api.patch<{ id: number; name: string; status: string }>(
          "/items/1",
          { status: "active" }
        );

        expect(data).toEqual({ id: 1, name: "Original", status: "active" });
      });

      it("makes PATCH request without body", async () => {
        server.use(
          http.patch(`${API_URL}/items/1/touch`, () => {
            return HttpResponse.json({ touched: true });
          })
        );

        const data = await api.patch<{ touched: boolean }>("/items/1/touch");

        expect(data.touched).toBe(true);
      });
    });

    describe("api.delete", () => {
      it("makes DELETE request and returns parsed JSON", async () => {
        server.use(
          http.delete(`${API_URL}/items/1`, () => {
            return HttpResponse.json({ deleted: true });
          })
        );

        const data = await api.delete<{ deleted: boolean }>("/items/1");

        expect(data.deleted).toBe(true);
      });

      it("handles 204 No Content response", async () => {
        server.use(
          http.delete(`${API_URL}/items/1`, () => {
            return new HttpResponse(null, { status: 204 });
          })
        );

        const data = await api.delete("/items/1");

        expect(data).toBeUndefined();
      });

      it("passes options to apiCall", async () => {
        server.use(
          http.delete(`${API_URL}/items`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              ids: url.searchParams.get("ids"),
            });
          })
        );

        const data = await api.delete<{ ids: string }>("/items", {
          params: { ids: "1,2,3" },
        });

        expect(data.ids).toBe("1,2,3");
      });
    });
  });
});
