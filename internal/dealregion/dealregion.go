// Package dealregion holds the set of region codes supported by psprices.com
// (ISO 3166-1 alpha-2, lowercase), used to validate the per-user deal_region
// preference and build psprices deal-search deep links.
package dealregion

// codes is the psprices region switcher set (scraped 2026-06-08).
var codes = map[string]bool{
	"ae": true, "ar": true, "at": true, "au": true, "be": true, "bg": true,
	"bh": true, "bo": true, "br": true, "ca": true, "ch": true, "cl": true,
	"cn": true, "co": true, "cr": true, "cy": true, "cz": true, "de": true,
	"dk": true, "ec": true, "es": true, "fi": true, "fr": true, "gb": true,
	"ge": true, "gr": true, "gt": true, "hk": true, "hn": true, "hr": true,
	"hu": true, "id": true, "ie": true, "il": true, "in": true, "iq": true,
	"is": true, "it": true, "jp": true, "kr": true, "kw": true, "kz": true,
	"lb": true, "lu": true, "mt": true, "mx": true, "my": true, "ni": true,
	"nl": true, "no": true, "nz": true, "om": true, "pa": true, "pe": true,
	"ph": true, "pk": true, "pl": true, "pt": true, "py": true, "qa": true,
	"ro": true, "ru": true, "sa": true, "se": true, "sg": true, "si": true,
	"sk": true, "sv": true, "th": true, "tr": true, "tw": true, "ua": true,
	"us": true, "uy": true, "vn": true, "za": true,
}

// Valid reports whether code is a supported psprices region code.
func Valid(code string) bool { return codes[code] }
