import type { UserGame } from './game';

/** One row in the pools index (GET /api/pools). */
export interface PoolListItem {
  id: string;
  name: string;
  color: string | null;
  position: number;
  has_filter: boolean;
  queue_count: number;
  candidate_count: number;
}

/** A single faceted filter card. Mirrors internal/filter/pool.go FilterCard. */
export interface FilterCard {
  play_status?: string;
  genre?: string[];
  theme?: string[];
  tag?: string[];
  platform?: string[];
  storefront?: string[];
  rating_min?: number;
  rating_max?: number;
  is_loved?: boolean;
  game_mode?: string[];
  player_perspective?: string[];
  q?: string;
  time_to_beat_min?: number;
  time_to_beat_max?: number;
}

/** A pool's saved filter: OR of cards. */
export interface PoolFilter {
  filters: FilterCard[];
}

/** Full pool (create/update/detail meta). `filter` is raw JSON from the API. */
export interface Pool {
  id: string;
  user_id: string;
  name: string;
  color: string | null;
  position: number;
  filter: PoolFilter | null;
  has_filter: boolean;
  created_at: string;
  updated_at: string;
}

/** GET /api/pools/:id — pool meta plus pre-split members. */
export interface PoolDetail extends Pool {
  queue: UserGame[];
  candidates: UserGame[];
}

/** One element of GET /api/pools/memberships. */
export interface PoolMembership {
  pool_id: string;
  position: number | null;
}
