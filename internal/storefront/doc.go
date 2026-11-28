// Package storefront owns a user's Connection to a storefront: the stored
// sign-in in user_sync_configs.storefront_credentials (ADR 0010).
//
// The stored value is the interface between "connect" and "sync". This
// package defines it once per storefront and is the one place it is
// encrypted and decrypted, with two exceptions left: the store-link resolver
// in cmd/nexorious reads the PlayStation Store sign-in itself, through
// PlayStationStoreCredentials (#1487), and the Amazon disconnect handler
// decrypts the sign-in Take returns, to sign the device out (#1485).
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
// ConnectPlayStationStore, ConnectGOG, ConnectHumbleBundle). Epic Games Store
// and Amazon Games still connect in internal/api (#1485). What is the same
// for every storefront is shared: Status reports a Connection with its
// account's display name, decoded from the stored sign-in, and Disconnect
// removes it.
//
// Below that, Connections stores, takes and loads the stored sign-in. Store
// also clears credentials_error, so a reconnect heals a sign-in a sync found
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
