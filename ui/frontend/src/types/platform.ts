export interface Platform {
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront?: string;
  storefronts?: Storefront[];
  created_at: string;
  updated_at: string;
}

export interface Storefront {
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
