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
Sanitised `helm.sh/chart` label value. Mirrors the standard `helm create`
output: replaces "+" (legal in semver build metadata but illegal in a
Kubernetes label value) with "_", caps at 63 chars, and strips a trailing
"-". Needed because Flux's source-controller appends the OCI artifact
digest as build metadata (e.g. `0.0.0-dev-20260520-abc+93a225106d8d`),
which the API server otherwise rejects.
*/}}
{{- define "nexorious.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Returns "true" when the managed credentials Secret has at least one
inline credential to write; empty string otherwise. When every credential
source is external (`*From` fields all set) and the bundled Postgres
controller is disabled, the Secret would otherwise render with metadata
but no `stringData` entries — pointless and noisy. Wrap the entire
credentials-secret.yaml body in `{{- if include "nexorious.credentialsSecretNeeded" . }}`.
*/}}
{{- define "nexorious.credentialsSecretNeeded" -}}
{{- $secretKeyFrom := default dict .Values.nexorious.secretKeyFrom -}}
{{- $igdbClientIdFrom := default dict .Values.nexorious.igdbClientIdFrom -}}
{{- $igdbClientSecretFrom := default dict .Values.nexorious.igdbClientSecretFrom -}}
{{- $databaseUrlFrom := default dict .Values.nexorious.databaseUrlFrom -}}
{{- $dbHostFrom := default dict .Values.nexorious.dbHostFrom -}}
{{- $dbPortFrom := default dict .Values.nexorious.dbPortFrom -}}
{{- $dbUserFrom := default dict .Values.nexorious.dbUserFrom -}}
{{- $dbPasswordFrom := default dict .Values.nexorious.dbPasswordFrom -}}
{{- $dbNameFrom := default dict .Values.nexorious.dbNameFrom -}}
{{- $pgUsernameFrom := default dict .Values.nexorious.postgresql.usernameFrom -}}
{{- $pgPasswordFrom := default dict .Values.nexorious.postgresql.passwordFrom -}}
{{- $pgDatabaseFrom := default dict .Values.nexorious.postgresql.databaseFrom -}}
{{- $omitDbUrl := or $databaseUrlFrom.name $dbHostFrom.name $dbPortFrom.name $dbUserFrom.name $dbPasswordFrom.name $dbNameFrom.name -}}
{{- $pgEnabled := dig "postgresql" "enabled" true .Values.controllers -}}
{{- if or
      (not $secretKeyFrom.name)
      (not $igdbClientIdFrom.name)
      (not $igdbClientSecretFrom.name)
      (and $pgEnabled (or (not $pgUsernameFrom.name) (not $pgPasswordFrom.name) (not $pgDatabaseFrom.name)))
      (not $omitDbUrl) -}}
true
{{- end -}}
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
  (dict "label" "secretKeyFrom"               "from" .Values.nexorious.secretKeyFrom)
  (dict "label" "igdbClientIdFrom"            "from" .Values.nexorious.igdbClientIdFrom)
  (dict "label" "igdbClientSecretFrom"        "from" .Values.nexorious.igdbClientSecretFrom)
  (dict "label" "databaseUrlFrom"             "from" .Values.nexorious.databaseUrlFrom)
  (dict "label" "dbHostFrom"                  "from" .Values.nexorious.dbHostFrom)
  (dict "label" "dbPortFrom"                  "from" .Values.nexorious.dbPortFrom)
  (dict "label" "dbUserFrom"                  "from" .Values.nexorious.dbUserFrom)
  (dict "label" "dbPasswordFrom"              "from" .Values.nexorious.dbPasswordFrom)
  (dict "label" "dbNameFrom"                  "from" .Values.nexorious.dbNameFrom)
  (dict "label" "postgresql.usernameFrom"     "from" .Values.nexorious.postgresql.usernameFrom)
  (dict "label" "postgresql.passwordFrom"     "from" .Values.nexorious.postgresql.passwordFrom)
  (dict "label" "postgresql.databaseFrom"     "from" .Values.nexorious.postgresql.databaseFrom)
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
{{/* Empty is fine — resolvePostgresPassword auto-generates and persists via
     lookup. We only reject the legacy literal placeholder in case someone
     copy-pastes it from older docs. */}}
{{- if $postgresqlEnabled -}}
  {{- $pw := .Values.nexorious.postgresql.password | default "" -}}
  {{- if eq $pw "change-me-in-production" -}}
    {{- fail "nexorious.postgresql.password is the literal placeholder. Leave empty to auto-generate, or set a real password (or use nexorious.postgresql.passwordFrom)." -}}
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
postgresql://{{ .Values.nexorious.postgresql.username }}:{{ .Values.nexorious.postgresql.password | urlquery }}@{{ include "nexorious.fullname" . }}-postgresql:5432/{{ .Values.nexorious.postgresql.database }}?sslmode=disable
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

{{/*
Helpers for the in-cluster Postgres pod's own credentials (POSTGRES_USER,
POSTGRES_PASSWORD, POSTGRES_DB). When nexorious.postgresql.*From.name is
set, the external secret is referenced; otherwise the managed credentials
secret is used, populated from the inline values.
*/}}

{{- define "nexorious.postgresUsernameSecretName" -}}
{{- if and .Values.nexorious.postgresql.usernameFrom .Values.nexorious.postgresql.usernameFrom.name -}}
{{- .Values.nexorious.postgresql.usernameFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.postgresUsernameSecretKey" -}}
{{- if and .Values.nexorious.postgresql.usernameFrom .Values.nexorious.postgresql.usernameFrom.key -}}
{{- .Values.nexorious.postgresql.usernameFrom.key -}}
{{- else -}}
POSTGRES_USER
{{- end -}}
{{- end }}

{{- define "nexorious.postgresPasswordSecretName" -}}
{{- if and .Values.nexorious.postgresql.passwordFrom .Values.nexorious.postgresql.passwordFrom.name -}}
{{- .Values.nexorious.postgresql.passwordFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.postgresPasswordSecretKey" -}}
{{- if and .Values.nexorious.postgresql.passwordFrom .Values.nexorious.postgresql.passwordFrom.key -}}
{{- .Values.nexorious.postgresql.passwordFrom.key -}}
{{- else -}}
POSTGRES_PASSWORD
{{- end -}}
{{- end }}

{{- define "nexorious.postgresDatabaseSecretName" -}}
{{- if and .Values.nexorious.postgresql.databaseFrom .Values.nexorious.postgresql.databaseFrom.name -}}
{{- .Values.nexorious.postgresql.databaseFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.postgresDatabaseSecretKey" -}}
{{- if and .Values.nexorious.postgresql.databaseFrom .Values.nexorious.postgresql.databaseFrom.key -}}
{{- .Values.nexorious.postgresql.databaseFrom.key -}}
{{- else -}}
POSTGRES_DB
{{- end -}}
{{- end }}

{{/*
Resolve .Values.nexorious.postgresql.password when in-cluster Postgres is
enabled, the field is empty, and no passwordFrom is configured. On first
install we generate a 32-char random password; on subsequent renders we
read it back from the existing managed Secret via `lookup` so it stays
stable. The resolved value is cached into .Values so the auto-built
DATABASE_URL and credentials-secret.yaml see the same string. Call BEFORE
bjw-s.common.loader.all and credentials-secret.yaml so both see the
resolved value.

Caveat: `helm template` and `helm install --dry-run` can't `lookup`, so
they emit a freshly generated password each time. Real `helm install`
and `helm upgrade` work correctly.
*/}}
{{- define "nexorious.resolvePostgresPassword" -}}
{{- $pgEnabled := dig "postgresql" "enabled" true .Values.controllers -}}
{{- if $pgEnabled -}}
  {{- $pgPasswordFromConfigured := not (empty (dig "name" "" (default dict .Values.nexorious.postgresql.passwordFrom))) -}}
  {{- if not $pgPasswordFromConfigured -}}
    {{- $current := .Values.nexorious.postgresql.password | default "" -}}
    {{- if eq $current "" -}}
      {{- $secretName := include "nexorious.credentialsSecretName" . -}}
      {{- $existing := lookup "v1" "Secret" .Release.Namespace $secretName -}}
      {{- $existingPw := "" -}}
      {{- if and $existing $existing.data -}}
        {{- $existingPw = index $existing.data "POSTGRES_PASSWORD" | default "" | b64dec -}}
      {{- end -}}
      {{- if $existingPw -}}
        {{- $_ := set .Values.nexorious.postgresql "password" $existingPw -}}
      {{- else -}}
        {{- $_ := set .Values.nexorious.postgresql "password" (randAlphaNum 32) -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end }}

{{/*
Apply .Values.global.storageClass to every enabled persistence entry that
does not already set its own storageClass. Call BEFORE bjw-s.common.loader.all
so the loader sees the mutated values.
*/}}
{{- define "nexorious.injectGlobalStorageClass" -}}
{{- $sc := dig "storageClass" "" (default dict .Values.global) -}}
{{- if $sc -}}
  {{- range $name, $pvc := .Values.persistence -}}
    {{- if and $pvc (kindIs "map" $pvc) (dig "enabled" false $pvc) -}}
      {{- if not (hasKey $pvc "storageClass") -}}
        {{- $_ := set $pvc "storageClass" $sc -}}
      {{- end -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end }}

{{/*
Inject legendary persistence entry and LEGENDARY_WORK_DIR env var when
nexorious.legendaryWorkDir is set. Call this BEFORE bjw-s.common.loader.all
so the loader sees the injected values.
*/}}
{{- define "nexorious.injectLegendaryIfEnabled" -}}
{{- if .Values.nexorious.legendaryWorkDir -}}
  {{- $_ := set .Values.persistence "legendary" (dict
      "enabled"        true
      "type"           "emptyDir"
      "advancedMounts" (dict
        "nexorious" (dict
          "main" (list (dict "path" .Values.nexorious.legendaryWorkDir))
        )
      )
  ) -}}
  {{- $_ = set .Values.controllers.nexorious.containers.main.env
      "LEGENDARY_WORK_DIR" .Values.nexorious.legendaryWorkDir -}}
{{- end -}}
{{- end }}

{{/*
Inject a writable /tmp emptyDir for the main container. Required because
controllers.nexorious.containers.main.securityContext.readOnlyRootFilesystem
is true and the app uses os.MkdirTemp("", ...) to stage backup contents
(pg_dump output + cover-art copy) and to buffer multipart restore uploads
before archiving/extracting. Call this BEFORE bjw-s.common.loader.all so
the loader sees the injected entry.
*/}}
{{- define "nexorious.injectTmpDir" -}}
  {{- $_ := set .Values.persistence "tmp" (dict
      "enabled"        true
      "type"           "emptyDir"
      "sizeLimit"      .Values.nexorious.tmpDir.sizeLimit
      "advancedMounts" (dict
        "nexorious" (dict
          "main" (list (dict "path" "/tmp"))
        )
      )
  ) -}}
{{- end }}
