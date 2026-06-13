package auth

// BcryptCost is the bcrypt cost factor used for every password hash in the
// application. It lives here so the API handlers and the reset-password CLI
// command share one source of truth.
//
// It is a var (not a const) solely so tests can lower it to bcrypt.MinCost in
// their TestMain — hashing at cost 12 hundreds of times dominates the test
// suite's runtime. Production code never reassigns it.
var BcryptCost = 12
