export interface User {
  id: string;
  username: string;
  // Note: API returns is_admin (snake_case), transformation to isAdmin (camelCase) needed
  isAdmin: boolean;
  preferences?: Record<string, unknown>;
}


export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
}

