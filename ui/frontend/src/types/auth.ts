export interface User {
  id: string;
  username: string;
  // Note: API returns is_admin (snake_case), transformation to isAdmin (camelCase) needed
  isAdmin: boolean;
  preferences?: Record<string, unknown>;
}
