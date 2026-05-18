{{/*
Expand the name of the chart.
*/}}
{{- define "nexorious.name" -}}
{{- include "bjw-s.common.lib.chart.names.name" . }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "nexorious.fullname" -}}
{{- include "bjw-s.common.lib.chart.names.fullname" . }}
{{- end }}

{{/*
Name of the credentials secret.
*/}}
{{- define "nexorious.credentialsSecretName" -}}
{{- include "bjw-s.common.lib.chart.names.fullname" . }}-credentials
{{- end }}

{{/*
Validate a single *From struct: both name and key must be set, or neither.
Input dict: .label (string), .from (object with .name and .key fields).
Returns an error string if invalid, empty string if valid.
Nil-safe: a nil .from is treated as an empty object.
*/}}
{{- define "nexorious._validateFrom" -}}
{{- $from := default dict .from -}}
{{- $name := dig "name" "" $from -}}
{{- $key := dig "key" "" $from -}}
{{- if and (not (empty $name)) (empty $key) -}}
{{- printf "%s: both name and key must be set, or neither" .label -}}
{{- else if and (empty $name) (not (empty $key)) -}}
{{- printf "%s: both name and key must be set, or neither" .label -}}
{{- end -}}
{{- end }}

{{/*
Validate required values.
*/}}
{{- define "nexorious.validateValues" -}}

{{/* --- *From completeness checks --- */}}
{{- $fromFields := list
  (dict "label" "secretKeyFrom"        "from" .Values.nexorious.secretKeyFrom)
  (dict "label" "igdbClientIdFrom"     "from" .Values.nexorious.igdbClientIdFrom)
  (dict "label" "igdbClientSecretFrom" "from" .Values.nexorious.igdbClientSecretFrom)
  (dict "label" "databaseUrlFrom"      "from" .Values.nexorious.databaseUrlFrom)
  (dict "label" "dbHostFrom"           "from" .Values.nexorious.dbHostFrom)
  (dict "label" "dbPortFrom"           "from" .Values.nexorious.dbPortFrom)
  (dict "label" "dbUserFrom"           "from" .Values.nexorious.dbUserFrom)
  (dict "label" "dbPasswordFrom"       "from" .Values.nexorious.dbPasswordFrom)
  (dict "label" "dbNameFrom"           "from" .Values.nexorious.dbNameFrom)
-}}
{{- range $fromFields -}}
  {{- $err := include "nexorious._validateFrom" . -}}
  {{- if not (empty $err) -}}
    {{- fail $err -}}
  {{- end -}}
{{- end -}}

{{/* --- Non-DB credential checks (bypass when *From is configured) --- */}}
{{- $secretKeyFromConfigured := not (empty (dig "name" "" (default dict .Values.nexorious.secretKeyFrom))) -}}
{{- if not $secretKeyFromConfigured -}}
  {{- if eq .Values.nexorious.secretKey "change-me-in-production" }}
    {{- fail "nexorious.secretKey must be set to a secure random value" }}
  {{- end }}
{{- end }}

{{- $igdbClientIdFromConfigured := not (empty (dig "name" "" (default dict .Values.nexorious.igdbClientIdFrom))) -}}
{{- if not $igdbClientIdFromConfigured -}}
  {{- if empty .Values.nexorious.igdbClientId }}
    {{- fail "nexorious.igdbClientId is required. nexorious will not function without valid IGDB credentials." }}
  {{- end }}
{{- end }}

{{- $igdbClientSecretFromConfigured := not (empty (dig "name" "" (default dict .Values.nexorious.igdbClientSecretFrom))) -}}
{{- if not $igdbClientSecretFromConfigured -}}
  {{- if empty .Values.nexorious.igdbClientSecret }}
    {{- fail "nexorious.igdbClientSecret is required. nexorious will not function without valid IGDB credentials." }}
  {{- end }}
{{- end }}

{{/* --- DB mode exclusivity check --- */}}
{{/* Mode A: inline databaseUrl */}}
{{- $modeA := not (empty .Values.nexorious.databaseUrl) -}}
{{/* Mode B: databaseUrlFrom */}}
{{- $modeB := not (empty (dig "name" "" (default dict .Values.nexorious.databaseUrlFrom))) -}}
{{/* Mode C: any individual db*From field */}}
{{- $modeC := or
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbHostFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbPortFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbUserFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbPasswordFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbNameFrom))))
-}}
{{- $activeModes := 0 -}}
{{- if $modeA }}{{- $activeModes = add $activeModes 1 }}{{- end -}}
{{- if $modeB }}{{- $activeModes = add $activeModes 1 }}{{- end -}}
{{- if $modeC }}{{- $activeModes = add $activeModes 1 }}{{- end -}}
{{- if gt $activeModes 1 -}}
  {{- fail "DB connection: at most one of databaseUrl, databaseUrlFrom, or individual db*From fields may be configured" }}
{{- end -}}

{{/* --- individual db*From: all-or-nothing check --- */}}
{{/* modeC is true when ANY db*From.name is set. All five must be set together. */}}
{{- $allDbIndividual := and
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbHostFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbPortFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbUserFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbPasswordFrom))))
  (not (empty (dig "name" "" (default dict .Values.nexorious.dbNameFrom))))
-}}
{{- if and $modeC (not $allDbIndividual) -}}
  {{- fail "db*From individual-keys mode: all five fields (dbHostFrom, dbPortFrom, dbUserFrom, dbPasswordFrom, dbNameFrom) must be configured together" }}
{{- end -}}

{{/* --- postgresql-disabled guard --- */}}
{{- $postgresqlEnabled := dig "postgresql" "enabled" true .Values.controllers -}}
{{- if not $postgresqlEnabled -}}
  {{- if not (or $modeA $modeB $modeC) -}}
    {{- fail "postgresql is disabled but no database configuration provided" }}
  {{- end -}}
{{- end -}}

{{/* --- postgresql password placeholder check (in-cluster only) --- */}}
{{- if $postgresqlEnabled -}}
  {{- $dbPasswordFromConfigured := not (empty (dig "name" "" (default dict .Values.nexorious.dbPasswordFrom))) -}}
  {{- $pw := .Values.nexorious.postgresql.password | default "" -}}
  {{- if and (eq $pw "change-me-in-production") (not $dbPasswordFromConfigured) -}}
    {{- fail "nexorious.postgresql.password must be set when using in-cluster Postgres (or use nexorious.dbPasswordFrom)" -}}
  {{- end -}}
{{- end -}}

{{- end }}

{{/*
Compute the database URL.
Uses nexorious.databaseUrl if set; otherwise builds in-cluster URL.
The auto-built URL URL-encodes the password so special characters
(: @ / ? # etc.) are safe.
*/}}
{{- define "nexorious.databaseUrl" -}}
{{- if .Values.nexorious.databaseUrl -}}
{{- .Values.nexorious.databaseUrl -}}
{{- else -}}
postgresql://{{ .Values.nexorious.postgresql.username }}:{{ .Values.nexorious.postgresql.password | urlquery }}@{{ include "nexorious.fullname" . }}-postgresql:5432/{{ .Values.nexorious.postgresql.database }}
{{- end -}}
{{- end }}

{{/*
Helpers for resolving secret sources via the *From pattern.
Each credential has two helpers: SecretName and SecretKey.
When *From.name is non-empty, the external secret is used.
Otherwise, falls back to the managed credentials secret.
*/}}

{{- define "nexorious.secretKeySecretName" -}}
{{- if and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.name -}}
{{- .Values.nexorious.secretKeyFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.secretKeySecretKey" -}}
{{- if and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.key -}}
{{- .Values.nexorious.secretKeyFrom.key -}}
{{- else -}}
SECRET_KEY
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientIdSecretName" -}}
{{- if and .Values.nexorious.igdbClientIdFrom .Values.nexorious.igdbClientIdFrom.name -}}
{{- .Values.nexorious.igdbClientIdFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientIdSecretKey" -}}
{{- if and .Values.nexorious.igdbClientIdFrom .Values.nexorious.igdbClientIdFrom.key -}}
{{- .Values.nexorious.igdbClientIdFrom.key -}}
{{- else -}}
IGDB_CLIENT_ID
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientSecretSecretName" -}}
{{- if and .Values.nexorious.igdbClientSecretFrom .Values.nexorious.igdbClientSecretFrom.name -}}
{{- .Values.nexorious.igdbClientSecretFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientSecretSecretKey" -}}
{{- if and .Values.nexorious.igdbClientSecretFrom .Values.nexorious.igdbClientSecretFrom.key -}}
{{- .Values.nexorious.igdbClientSecretFrom.key -}}
{{- else -}}
IGDB_CLIENT_SECRET
{{- end -}}
{{- end }}

{{- define "nexorious.databaseUrlSecretName" -}}
{{- if and .Values.nexorious.databaseUrlFrom .Values.nexorious.databaseUrlFrom.name -}}
{{- .Values.nexorious.databaseUrlFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.databaseUrlSecretKey" -}}
{{- if and .Values.nexorious.databaseUrlFrom .Values.nexorious.databaseUrlFrom.key -}}
{{- .Values.nexorious.databaseUrlFrom.key -}}
{{- else -}}
DATABASE_URL
{{- end -}}
{{- end }}

{{- define "nexorious.dbHostSecretName" -}}
{{- if and .Values.nexorious.dbHostFrom .Values.nexorious.dbHostFrom.name -}}
{{- .Values.nexorious.dbHostFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbHostSecretKey" -}}
{{- if and .Values.nexorious.dbHostFrom .Values.nexorious.dbHostFrom.key -}}
{{- .Values.nexorious.dbHostFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbPortSecretName" -}}
{{- if and .Values.nexorious.dbPortFrom .Values.nexorious.dbPortFrom.name -}}
{{- .Values.nexorious.dbPortFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbPortSecretKey" -}}
{{- if and .Values.nexorious.dbPortFrom .Values.nexorious.dbPortFrom.key -}}
{{- .Values.nexorious.dbPortFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbUserSecretName" -}}
{{- if and .Values.nexorious.dbUserFrom .Values.nexorious.dbUserFrom.name -}}
{{- .Values.nexorious.dbUserFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbUserSecretKey" -}}
{{- if and .Values.nexorious.dbUserFrom .Values.nexorious.dbUserFrom.key -}}
{{- .Values.nexorious.dbUserFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbPasswordSecretName" -}}
{{- if and .Values.nexorious.dbPasswordFrom .Values.nexorious.dbPasswordFrom.name -}}
{{- .Values.nexorious.dbPasswordFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbPasswordSecretKey" -}}
{{- if and .Values.nexorious.dbPasswordFrom .Values.nexorious.dbPasswordFrom.key -}}
{{- .Values.nexorious.dbPasswordFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbNameSecretName" -}}
{{- if and .Values.nexorious.dbNameFrom .Values.nexorious.dbNameFrom.name -}}
{{- .Values.nexorious.dbNameFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbNameSecretKey" -}}
{{- if and .Values.nexorious.dbNameFrom .Values.nexorious.dbNameFrom.key -}}
{{- .Values.nexorious.dbNameFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}
