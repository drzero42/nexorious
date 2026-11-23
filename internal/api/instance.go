package api

import "crypto/rand"

// instanceID identifies this server process. It is new on every process start,
// so a client that triggered a restore (which ends with the server restarting
// itself, #1341) can tell the old process from the new one by polling /health
// until the id changes. It carries no meaning beyond that and is not secret.
var instanceID = rand.Text()
