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
Validate required values.
*/}}
{{- define "nexorious.validateValues" -}}
{{- if eq .Values.nexorious.secretKey "change-me-in-production" }}
  {{- fail "nexorious.secretKey must be set to a secure random value" }}
{{- end }}
{{- if eq .Values.nexorious.internalApiKey "change-me-in-production" }}
  {{- fail "nexorious.internalApiKey must be set to a secure random value" }}
{{- end }}
{{- if and (not (dig "postgresql" "enabled" true .Values.controllers)) (empty .Values.nexorious.databaseUrl) }}
  {{- fail "nexorious.databaseUrl must be set when the postgresql controller is disabled" }}
{{- end }}
{{- if and (not (dig "nats" "enabled" true .Values.controllers)) (empty .Values.nexorious.natsUrl) }}
  {{- fail "nexorious.natsUrl must be set when the nats controller is disabled" }}
{{- end }}
{{- if empty .Values.nexorious.igdbClientId }}
  {{- fail "nexorious.igdbClientId is required. Nexorious will not function without valid IGDB credentials." }}
{{- end }}
{{- if empty .Values.nexorious.igdbClientSecret }}
  {{- fail "nexorious.igdbClientSecret is required. Nexorious will not function without valid IGDB credentials." }}
{{- end }}
{{- end }}

{{/*
Compute the database URL.
Uses nexorious.databaseUrl if set; otherwise builds in-cluster URL.
NOTE: The auto-built URL does not URL-encode the password. If the password
contains special characters (: @ / ? # etc.), set nexorious.databaseUrl
explicitly with the password pre-encoded.
*/}}
{{- define "nexorious.databaseUrl" -}}
{{- if .Values.nexorious.databaseUrl -}}
{{- .Values.nexorious.databaseUrl -}}
{{- else -}}
postgresql://{{ .Values.nexorious.postgresql.username }}:{{ .Values.nexorious.postgresql.password }}@{{ include "nexorious.fullname" . }}-postgresql:5432/{{ .Values.nexorious.postgresql.database }}
{{- end -}}
{{- end }}

{{/*
Compute the NATS URL.
Uses nexorious.natsUrl if set; otherwise builds in-cluster URL.
*/}}
{{- define "nexorious.natsUrl" -}}
{{- if .Values.nexorious.natsUrl -}}
{{- .Values.nexorious.natsUrl -}}
{{- else -}}
nats://{{ include "nexorious.fullname" . }}-nats:4222
{{- end -}}
{{- end }}

{{/*
Compute the internal API URL used by worker/scheduler.
Uses nexorious.internalApiUrl if set; otherwise builds in-cluster URL.
*/}}
{{- define "nexorious.internalApiUrl" -}}
{{- if .Values.nexorious.internalApiUrl -}}
{{- .Values.nexorious.internalApiUrl -}}
{{- else -}}
http://{{ include "nexorious.fullname" . }}-api:8000
{{- end -}}
{{- end }}

{{/*
Helpers for resolving secret sources via the *From pattern.
Each credential has two helpers: SecretName and SecretKey.
When *From.name is non-empty, the external secret is used.
Otherwise, falls back to the managed nexorious-credentials secret.
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

{{- define "nexorious.internalApiKeySecretName" -}}
{{- if and .Values.nexorious.internalApiKeyFrom .Values.nexorious.internalApiKeyFrom.name -}}
{{- .Values.nexorious.internalApiKeyFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.internalApiKeySecretKey" -}}
{{- if and .Values.nexorious.internalApiKeyFrom .Values.nexorious.internalApiKeyFrom.key -}}
{{- .Values.nexorious.internalApiKeyFrom.key -}}
{{- else -}}
INTERNAL_API_KEY
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
