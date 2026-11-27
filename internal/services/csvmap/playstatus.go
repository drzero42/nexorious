package csvmap

import "github.com/drzero42/nexorious/internal/enum"

// Play statuses as plain strings: the mapping tables and Config fields hold
// play_status as string, so these keep the presets short while still reading
// every value from internal/enum.
const (
	psNotStarted = string(enum.PlayStatusNotStarted)
	psInProgress = string(enum.PlayStatusInProgress)
	psCompleted  = string(enum.PlayStatusCompleted)
	psMastered   = string(enum.PlayStatusMastered)
	psDominated  = string(enum.PlayStatusDominated)
	psShelved    = string(enum.PlayStatusShelved)
	psDropped    = string(enum.PlayStatusDropped)
	psReplay     = string(enum.PlayStatusReplay)
)
