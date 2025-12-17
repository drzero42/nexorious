export interface User {
  id: string;
  username: string;
  isAdmin: boolean;
  preferences?: Record<string, unknown>;
}

export interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  isLoading: boolean;
  error: string | null;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export interface SetupStatusResponse {
  needs_setup: boolean;
}

export interface CreateAdminRequest {
  username: string;
  password: string;
}
