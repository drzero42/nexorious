# internal/storefront owns storefront Connections

A user's Connection to a storefront is the sign-in stored, encrypted, in `user_sync_configs.storefront_credentials`. Its JSON is the interface between "connect" and "sync", and it had no shared type. The connect handlers in `internal/api/sync.go` wrote it as map literals, and three other places read it back through their own anonymous structs: the sync adapter factory in `cmd/nexorious/serve.go`, the store-link resolver factory, and the GET connection handlers. The persistence (store, clear, take, load) sat in the HTTP handler, and saving a sign-in a sync refreshed sat in `cmd/nexorious`. The layout had a concrete risk: the GOG refresh re-marshalled the stored JSON from a struct that knew two fields, so a field connect started storing would be dropped by the first token refresh.

We decided that `internal/storefront` owns the Connection (#1411, #1483):

- **The stored shape**, defined once per storefront. Steam, PlayStation Store, GOG and Humble Bundle have exported typed credentials that the connect handlers store and every reader decodes. Epic's legendary snapshot and Amazon's nile user data stay opaque: their service packages own those shapes, and the module stores and loads them unchanged.
- **Persistence**: `Connections` stores (clearing `credentials_error`), clears, takes and loads a Connection.
- **Building the library adapter** from a stored Connection, through a registry with one definition per storefront. The shared load path (no sign-in, a failed decrypt, an undecodable value: `ErrCredentials`) is written once.
- **Saving a refreshed sign-in**, only while the stored ciphertext is still the one the sync last read or wrote, in one `UPDATE`, and binding a credentials failure to that ciphertext (`storefrontadapter.CredentialsErrorFor`).

The module imports the storefront service packages, `storefrontadapter`, `internal/crypto` and `internal/db/models`, and neither `internal/api`, `internal/worker/tasks` nor `internal/joblifecycle`. The sync worker keeps taking its adapter factory as an injected function; `serve.go` passes the module's.

## Considered options

- **Typed credentials in the service packages.** Rejected: the stored shape is the module's concern, not the storefront API's. The service adapters take plain strings and stay unaware of how the sign-in is stored.
- **Typing Epic and Amazon too.** Rejected: their stored value is whatever legendary and nile produce, and a typed copy would drop fields those tools add.

## Consequences

- **Moved in stages.** This first slice moves the stored shape, persistence and adapter building. The connect, status and disconnect handlers stay in `internal/api` and only call the module (#1485). The store-link resolver factory stays in `serve.go` but decodes PlayStation credentials through the module's type (#1487). The Amazon disconnect still decrypts the sign-in `Take` returns, in its background device sign-out, until the disconnect handlers move in (since moved; see the update below). No guard test checks the ownership yet; #1487 adds one.
- **Outside by design:** `credentials_error` is storefront health (#1410). `Store` still clears it, as connecting did before, and the worker's `flagCredentialsError` still compares `storefront_credentials` in its `UPDATE`; whichever of #1410 and the later slices lands second replaces those direct column writes with a call.
- **Compatible with stored data:** the JSON field names are unchanged and decoding ignores unknown fields (older PlayStation rows carry four nobody reads), so existing Connections keep working without reconnecting.

## Update: connecting moved in (#1485)

The connect, status and disconnect flows are now the module's too. Each storefront has its own connect operation (`ConnectSteam`, `ConnectPlayStationStore`, `ConnectGOG`, `ConnectHumbleBundle`, `ConnectEpicGamesStore`, and Amazon's two-step `StartAmazonGamesSignIn` and `ConnectAmazonGames`); there is no common one. Each operation validates its input and returns typed errors, and the handlers in `internal/api` map those to the unchanged HTTP responses. Epic's legendary cleanup and Amazon's background device sign-out are part of their storefront's disconnect (`DisconnectEpicGamesStore`, `DisconnectAmazonGames`). Amazon's sealed sign-in attempt moved unchanged. With that, the module is the only place a stored sign-in is decrypted, apart from the store-link resolver (#1487), and the handler no longer holds the encrypter.
