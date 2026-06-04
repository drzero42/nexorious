package platformresolution

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
	case "humble-bundle":
		return "humble-bundle", true
	default:
		return "", false
	}
}
