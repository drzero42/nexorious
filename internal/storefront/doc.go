// Package storefront owns a user's Connection to a storefront: the stored
// sign-in in user_sync_configs.storefront_credentials (ADR 0010).
//
// The stored value is the interface between "connect" and "sync". This
// package defines it once per storefront and is the one place it is
// encrypted and decrypted, with one exception left: the store-link resolver
// in cmd/nexorious reads the PlayStation Store sign-in itself, through
// PlayStationStoreCredentials (#1487).
//
// The stored shapes:
//
//   - Steam, PlayStation Store, GOG and Humble Bundle store a typed value
//     (SteamCredentials and friends). Connect stores it, and every reader
//     decodes it, through that one type, so a field connect starts storing
//     survives a sync's refresh. Unknown fields in older rows are ignored on
//     decode.
//   - Epic Games Store and Amazon Games store an opaque value the service
//     package owns: legendary's state snapshot and nile's user data. This
//     package stores and loads them unchanged.
//
// Connections connects, reports and disconnects a Connection, reaching the
// storefronts through the service clients it is built with (Clients), so
// tests stand fakes in for the third parties. There is no common connect
// operation: each storefront's flow differs (a key and an id, a token, a
// cookie, an auth code, a two-step sign-in), so each gets its own, which
// validates its input and returns typed errors, never HTTP ones (ConnectSteam,
// ConnectPlayStationStore, ConnectGOG, ConnectHumbleBundle,
// ConnectEpicGamesStore, and Amazon's StartAmazonGamesSignIn then
// ConnectAmazonGames). What is the same for every storefront is shared:
// Status reports a Connection with its account's display name, decoded from
// the stored sign-in, and Disconnect removes it. Two storefronts tear down
// more on disconnect, best effort: DisconnectEpicGamesStore removes the
// legendary files older versions left on disk, and DisconnectAmazonGames
// signs the removed sign-in's device out of Amazon in the background.
//
// Amazon's sign-in takes two requests. The values nile needs to finish it
// never reach the browser in the clear: they travel in the attempt id,
// encrypted with the server's key, marked as an Amazon sign-in attempt, bound
// to the user who started it and valid for 15 minutes, so a start and a
// finish may reach different servers.
//
// Below that, Connections stores and loads the stored sign-in. Store also
// clears credentials_error, so a reconnect heals a sign-in a sync found
// expired. Load hands back the sign-in decrypted.
//
// A registry holds one definition per storefront, in the order the sync API
// lists them (Slugs). Through it, Connections.AdapterFactory builds a
// storefront's library adapter from a stored Connection.
// A missing, undecryptable or undecodable sign-in gives
// storefrontadapter.ErrCredentials. A sign-in the sync refreshed (GOG's
// refresh token, Epic's snapshot, Amazon's user data) is saved only while the
// stored ciphertext is still the one the sync last read or wrote, in one
// UPDATE, so a reconnect or disconnect during the sync wins. A credentials
// failure from the adapter is returned as a
// storefrontadapter.CredentialsErrorFor bound to that same ciphertext.
//
// The package imports the storefront service packages, storefrontadapter,
// crypto and models; it imports neither internal/api, internal/worker/tasks
// nor internal/joblifecycle.
package storefront
