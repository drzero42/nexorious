package auth

// BcryptCost is the bcrypt cost factor used for every password hash in the
// application. It lives here so the API handlers and the reset-password CLI
// command share one source of truth.
const BcryptCost = 12
