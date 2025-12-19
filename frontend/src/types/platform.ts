export interface Platform {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront_id?: string;
  storefronts?: Storefront[];
  created_at: string;
  updated_at: string;
}

export interface Storefront {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}

export interface PlatformsResponse {
  platforms: Platform[];
}

export interface StorefrontsResponse {
  storefronts: Storefront[];
}
