package api

import "fmt"

// buildStoreURL turns a (storefront, store_link) pair into a product-page URL.
// It returns ("", false) when no reliable link can be built — an empty
// store_link, a storefront with no URL scheme (humble-bundle), or an unknown
// storefront. URL formats live here so a store changing its scheme is a code
// fix, not a re-sync. Storefront keys are the canonical storefronts.name slugs.
func buildStoreURL(storefront, storeLink string) (string, bool) {
	if storeLink == "" {
		return "", false
	}
	switch storefront {
	case "steam":
		return fmt.Sprintf("https://store.steampowered.com/app/%s/", storeLink), true
	case "gog":
		return fmt.Sprintf("https://www.gog.com/game/%s", storeLink), true
	case "epic-games-store":
		return fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", storeLink), true
	case "playstation-store":
		return fmt.Sprintf("https://store.playstation.com/en-us/concept/%s", storeLink), true
	default:
		return "", false
	}
}
