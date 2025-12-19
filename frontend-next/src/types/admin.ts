export interface AdminUser {
  id: string;
  username: string;
  isAdmin: boolean;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AdminUserBackend {
  id: string;
  username: string;
  is_admin: boolean;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface UserDeletionImpact {
  user_id: string;
  username: string;
  total_games: number;
  total_tags: number;
  total_wishlist_items: number;
  total_import_jobs: number;
  total_sessions: number;
  warning: string;
}

export interface CreateUserRequest {
  username: string;
  password: string;
  is_admin?: boolean;
}

export interface UpdateUserRequest {
  username?: string;
  is_active?: boolean;
  is_admin?: boolean;
}

export interface ResetPasswordRequest {
  new_password: string;
}
