package platformresolution

// PlatformToSlug maps a raw platform string from a sync adapter to a canonical platforms.name slug.
// Returns ("", false) for unknown raw platforms.
func PlatformToSlug(raw string) (string, bool) {
	switch raw {
	case "pc-windows":
		return "pc-windows", true
	case "pc-linux":
		return "pc-linux", true
	case "pc-mac":
		return "mac", true
	case "playstation-5":
		return "playstation-5", true
	case "playstation-4":
		return "playstation-4", true
	default:
		return "", false
	}
}

// StorefrontToCollectionSlug maps a sync-source storefront identifier to the storefronts.name slug.
// Returns ("", false) for storefronts with no collection mapping (epic, gog, etc.).
func StorefrontToCollectionSlug(storefront string) (string, bool) {
	switch storefront {
	case "steam":
		return "steam", true
	case "psn":
		return "playstation-store", true
	case "epic":
		return "epic-games-store", true
	case "gog":
		return "gog", true
	default:
		return "", false
	}
}
