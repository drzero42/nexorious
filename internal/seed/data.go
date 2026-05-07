package seed

// StorefrontSeed holds data for a single official storefront row.
type StorefrontSeed struct {
	Name         string
	DisplayName  string
	IconURL      string
	BaseURL      string
	IsActive     bool
	VersionAdded string
}

// PlatformSeed holds data for a single official platform row.
type PlatformSeed struct {
	Name                  string
	DisplayName           string
	IconURL               string
	IsActive              bool
	VersionAdded          string
	DefaultStorefront     *string // nullable FK to storefronts.name
	IGDBPlatformID        *int    // reserved for future use
	IGDBPlatformVersionID *int    // reserved for future use
}

// AssociationSeed holds a platform↔storefront link.
type AssociationSeed struct {
	Platform   string
	Storefront string
}

func strPtr(s string) *string { return &s }

// OfficialStorefronts is the canonical list of official storefronts.
var OfficialStorefronts = []StorefrontSeed{
	{Name: "steam", DisplayName: "Steam", IconURL: "/static/logos/storefronts/steam/steam-icon-light.svg", BaseURL: "https://store.steampowered.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "epic-games-store", DisplayName: "Epic Games Store", IconURL: "/static/logos/storefronts/epic-games-store/epic-games-store-icon-light.svg", BaseURL: "https://store.epicgames.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "gog", DisplayName: "GOG", IconURL: "/static/logos/storefronts/gog/gog-icon-light.svg", BaseURL: "https://www.gog.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "playstation-store", DisplayName: "PlayStation Store", IconURL: "/static/logos/storefronts/playstation-store/playstation-store-icon-light.svg", BaseURL: "https://store.playstation.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "microsoft-store", DisplayName: "Microsoft Store", IconURL: "/static/logos/storefronts/microsoft-store/microsoft-store-icon-light.svg", BaseURL: "https://www.microsoft.com/store", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "nintendo-eshop", DisplayName: "Nintendo eShop", IconURL: "/static/logos/storefronts/nintendo-eshop/nintendo-eshop-icon-light.svg", BaseURL: "https://www.nintendo.com/us/store", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "itch-io", DisplayName: "Itch.io", IconURL: "/static/logos/storefronts/itch-io/itch-io-icon-light.svg", BaseURL: "https://itch.io", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "origin-ea-app", DisplayName: "Origin/EA App", IconURL: "/static/logos/storefronts/origin-ea-app/origin-ea-app-icon-light.svg", BaseURL: "https://www.ea.com/ea-app", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "apple-app-store", DisplayName: "Apple App Store", IconURL: "/static/logos/storefronts/apple-app-store/apple-app-store-icon-light.svg", BaseURL: "https://apps.apple.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "google-play-store", DisplayName: "Google Play Store", IconURL: "/static/logos/storefronts/google-play-store/google-play-store-icon-light.svg", BaseURL: "https://play.google.com/store", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "humble-bundle", DisplayName: "Humble Bundle", IconURL: "/static/logos/storefronts/humble-bundle/humble-bundle-icon-light.svg", BaseURL: "https://www.humblebundle.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "physical", DisplayName: "Physical", IconURL: "/static/logos/storefronts/physical/physical-icon-light.svg", BaseURL: "", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "uplay", DisplayName: "UPlay", IconURL: "/static/logos/storefronts/uplay/uplay-icon-light.svg", BaseURL: "https://store.ubi.com", IsActive: true, VersionAdded: "1.0.0"},
	{Name: "gamersgate", DisplayName: "GamersGate", IconURL: "/static/logos/storefronts/gamersgate/gamersgate-icon-light.svg", BaseURL: "https://www.gamersgate.com", IsActive: true, VersionAdded: "1.0.0"},
}

// OfficialPlatforms is the canonical list of official platforms.
var OfficialPlatforms = []PlatformSeed{
	{Name: "pc-windows", DisplayName: "PC (Windows)", IconURL: "/static/logos/platforms/pc-windows/pc-windows-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("steam")},
	{Name: "playstation-5", DisplayName: "PlayStation 5", IconURL: "/static/logos/platforms/playstation-5/playstation-5-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("playstation-store")},
	{Name: "playstation-4", DisplayName: "PlayStation 4", IconURL: "/static/logos/platforms/playstation-4/playstation-4-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("playstation-store")},
	{Name: "playstation-3", DisplayName: "PlayStation 3", IconURL: "/static/logos/platforms/playstation-3/playstation-3-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("playstation-store")},
	{Name: "playstation-vita", DisplayName: "PlayStation Vita", IconURL: "/static/logos/platforms/playstation-vita/playstation-vita-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("playstation-store")},
	{Name: "playstation-psp", DisplayName: "PlayStation Portable (PSP)", IconURL: "/static/logos/platforms/playstation-psp/playstation-psp-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("playstation-store")},
	{Name: "xbox-series", DisplayName: "Xbox Series X/S", IconURL: "/static/logos/platforms/xbox-series/xbox-series-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("microsoft-store")},
	{Name: "xbox-one", DisplayName: "Xbox One", IconURL: "/static/logos/platforms/xbox-one/xbox-one-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("microsoft-store")},
	{Name: "xbox-360", DisplayName: "Xbox 360", IconURL: "/static/logos/platforms/xbox-360/xbox-360-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("microsoft-store")},
	{Name: "nintendo-switch", DisplayName: "Nintendo Switch", IconURL: "/static/logos/platforms/nintendo-switch/nintendo-switch-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("nintendo-eshop")},
	{Name: "nintendo-wii", DisplayName: "Nintendo Wii", IconURL: "/static/logos/platforms/nintendo-wii/nintendo-wii-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("nintendo-eshop")},
	{Name: "ios", DisplayName: "iOS", IconURL: "/static/logos/platforms/ios/ios-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("apple-app-store")},
	{Name: "android", DisplayName: "Android", IconURL: "/static/logos/platforms/android/android-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("google-play-store")},
	{Name: "playstation-2", DisplayName: "PlayStation 2", IconURL: "/static/logos/platforms/playstation-2/playstation-2-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("physical")},
	{Name: "playstation", DisplayName: "PlayStation", IconURL: "/static/logos/platforms/playstation/playstation-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("physical")},
	{Name: "nintendo-wii-u", DisplayName: "Nintendo Wii U", IconURL: "/static/logos/platforms/nintendo-wii-u/nintendo-wii-u-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("nintendo-eshop")},
	{Name: "pc-linux", DisplayName: "PC (Linux)", IconURL: "/static/logos/platforms/pc-linux/pc-linux-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("steam")},
	{Name: "mac", DisplayName: "Mac", IconURL: "/static/logos/platforms/mac/mac-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("steam")},
	{Name: "nintendo-switch-2", DisplayName: "Nintendo Switch 2", IconURL: "/static/logos/platforms/nintendo-switch-2/nintendo-switch-2-icon-light.svg", IsActive: true, VersionAdded: "1.0.0", DefaultStorefront: strPtr("nintendo-eshop")},
}

// OfficialAssociations is the canonical list of platform↔storefront associations.
var OfficialAssociations = []AssociationSeed{
	// PC (Windows)
	{Platform: "pc-windows", Storefront: "steam"},
	{Platform: "pc-windows", Storefront: "epic-games-store"},
	{Platform: "pc-windows", Storefront: "gog"},
	{Platform: "pc-windows", Storefront: "origin-ea-app"},
	{Platform: "pc-windows", Storefront: "microsoft-store"},
	{Platform: "pc-windows", Storefront: "itch-io"},
	{Platform: "pc-windows", Storefront: "gamersgate"},
	{Platform: "pc-windows", Storefront: "physical"},
	// PlayStation 5
	{Platform: "playstation-5", Storefront: "playstation-store"},
	{Platform: "playstation-5", Storefront: "physical"},
	// PlayStation 4
	{Platform: "playstation-4", Storefront: "playstation-store"},
	{Platform: "playstation-4", Storefront: "physical"},
	// PlayStation 3
	{Platform: "playstation-3", Storefront: "playstation-store"},
	{Platform: "playstation-3", Storefront: "physical"},
	// PlayStation Vita
	{Platform: "playstation-vita", Storefront: "playstation-store"},
	{Platform: "playstation-vita", Storefront: "physical"},
	// PlayStation Portable (PSP)
	{Platform: "playstation-psp", Storefront: "playstation-store"},
	{Platform: "playstation-psp", Storefront: "physical"},
	// Xbox Series X/S
	{Platform: "xbox-series", Storefront: "microsoft-store"},
	{Platform: "xbox-series", Storefront: "physical"},
	// Xbox One
	{Platform: "xbox-one", Storefront: "microsoft-store"},
	{Platform: "xbox-one", Storefront: "physical"},
	// Xbox 360
	{Platform: "xbox-360", Storefront: "microsoft-store"},
	{Platform: "xbox-360", Storefront: "physical"},
	// Nintendo Switch
	{Platform: "nintendo-switch", Storefront: "nintendo-eshop"},
	{Platform: "nintendo-switch", Storefront: "physical"},
	// Nintendo Wii
	{Platform: "nintendo-wii", Storefront: "nintendo-eshop"},
	{Platform: "nintendo-wii", Storefront: "physical"},
	// iOS
	{Platform: "ios", Storefront: "apple-app-store"},
	{Platform: "ios", Storefront: "epic-games-store"},
	// Android
	{Platform: "android", Storefront: "google-play-store"},
	{Platform: "android", Storefront: "epic-games-store"},
	// PC (Linux)
	{Platform: "pc-linux", Storefront: "steam"},
	{Platform: "pc-linux", Storefront: "gog"},
	{Platform: "pc-linux", Storefront: "humble-bundle"},
	// PlayStation 2
	{Platform: "playstation-2", Storefront: "physical"},
	// PlayStation
	{Platform: "playstation", Storefront: "physical"},
	// Nintendo Wii U
	{Platform: "nintendo-wii-u", Storefront: "nintendo-eshop"},
	{Platform: "nintendo-wii-u", Storefront: "physical"},
	// Nintendo Switch 2
	{Platform: "nintendo-switch-2", Storefront: "nintendo-eshop"},
	{Platform: "nintendo-switch-2", Storefront: "physical"},
	// Mac
	{Platform: "mac", Storefront: "steam"},
}
