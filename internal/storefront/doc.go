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
//     (SteamCredentials and friends). The connect handler stores it, and every
//     reader decodes it, through that one type, so a field the connect handler
//     starts storing survives a sync's refresh. Unknown fields in older rows
//     are ignored on decode.
//   - Epic Games Store and Amazon Games store an opaque value the service
//     package owns: legendary's state snapshot and nile's user data. This
//     package stores and loads them unchanged.
//
// Connections stores, clears, takes and loads a Connection. Store also clears
// credentials_error, so a reconnect heals a sign-in a sync found expired.
// Load hands back the sign-in decrypted, and callers decode it into the
// storefront's credentials type.
//
// Connections.AdapterFactory builds a storefront's library adapter from a
// stored Connection, through a registry with one definition per storefront.
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
