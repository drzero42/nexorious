# Completionator CSV Import

This document is the source of truth for how Nexorious imports a game collection
from a **Completionator CSV export**. Completionator is an active game-tracking
service; this is a **one-off migration path**, not a recurring sync.

Completionator is supported as a `csvmap` **preset `Config`** (`csvmap.Completionator()`),
not a bespoke mapper. Identifying games requires IGDB; the import is blocked
unless IGDB is configured.

## The export format

A Completionator CSV is **one row per game** with a 24-column header:

```
Name, Edition, Platform, Format, Region, Now Playing, Backlogged,
Ownership Status, Progress Status, Est. Value, Amt. Paid, Tags, Box/Case,
Cart/Disc, Manual, Extras, Acquisition Type, Acquisition Source,
Acquisition Date, Rating, Initial Release Date, Item Release Date, Added On, Genre
```

### Two format quirks (handled by `csvmap.ReadRecords`)

1. **Malformed quoting.** Every field is quote-wrapped, but embedded quotes in
   titles are **not** RFC-4180-escaped — e.g.
   `"...Episode 1: "Done Running"",""...`. Strict `encoding/csv` rejects this.
   `ReadRecords` falls back to stripping the outer quotes and splitting each line
   on the literal `","` — but only when every line is uniformly quote-wrapped, so
   a genuinely malformed file errors rather than corrupting silently.
2. **Windows-1252 encoding.** Exports are Windows-1252, not UTF-8. `ReadRecords`
   transcodes to UTF-8 when the bytes are not already valid UTF-8, so accented
   titles import correctly.

## Mapping into Nexorious

| Nexorious field | Completionator column | Notes |
|---|---|---|
| Title | `Name` | required |
| Play status | `Progress Status` | `Finished` → completed, `Incomplete` → not_started (default) |
| Platform | `Platform` | `PC / Windows` → `pc-windows`, `PlayStation 5` → `playstation-5` |
| Storefront | `Format` | `Digital (Steam)` → `steam`, `Digital (GOG)` → `gog`, `Physical (*)` → `physical` |
| Acquired date | `Acquisition Date` | `M/D/YYYY` |
| Added date | `Added On` | `M/D/YYYY` |
| Rating | `Rating` | 1–10 scale → 1–5 stars, round-to-nearest |
| Tags | `Tags` | comma-separated |

Rows sharing a title are merged into one game (platform entries unioned).

### Deliberately not mapped

`Edition`, `Region`, `Now Playing` / `Backlogged` (see status note below),
`Est. Value`, `Amt. Paid`, `Box/Case`, `Cart/Disc`, `Manual`, `Extras`,
`Acquisition Type`, `Acquisition Source`, `Initial Release Date`,
`Item Release Date`, and `Genre` (IGDB re-supplies genre on match).

### Known limitations

- **Play status uses `Progress Status` only.** Completionator also has `Now
  Playing` and `Backlogged` flags; honouring their precedence would need the
  advanced `StatusFlags` engine feature (#1016). Until then, a "Now Playing"
  game imports as `not_started` rather than `in_progress`.
- **Platform / `Format` value maps are derived from observed exports.** An
  unmapped platform value imports the game without that platform (logged); an
  unmapped `Format` records the platform without a storefront. The maps are
  extensible as new values are confirmed.
