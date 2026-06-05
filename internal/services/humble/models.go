// Package humble implements a storefront sync adapter for Humble Bundle. It
// imports only DRM-free games downloaded directly from Humble — never ebooks,
// audio, video, or games for which only a third-party (Steam) key is granted.
package humble

// Order is one Humble order's detail (GET /api/v1/order/{gamekey}). Only the
// fields the adapter reads are modelled; tpkd_dict (third-party keys) is
// deliberately omitted so Steam-key-only titles are never imported.
type Order struct {
	Gamekey     string       `json:"gamekey"`
	Subproducts []Subproduct `json:"subproducts"`
}

// Subproduct is one item in an order: a game, a bundled ebook/audio/video, or a
// promo/info stub. Game-ness is decided by its downloads (see adapter.gameEntry).
type Subproduct struct {
	MachineName string     `json:"machine_name"`
	HumanName   string     `json:"human_name"`
	Downloads   []Download `json:"downloads"`
}

// Download is one platform-specific download for a subproduct.
type Download struct {
	Platform       string           `json:"platform"`
	MachineName    string           `json:"machine_name"`
	DownloadStruct []DownloadStruct `json:"download_struct"`
}

// DownloadStruct is one downloadable file within a Download. A non-empty
// URL.Web is what distinguishes a real download from an empty stub.
type DownloadStruct struct {
	URL struct {
		Web string `json:"web"`
	} `json:"url"`
}
