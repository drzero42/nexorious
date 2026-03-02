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
{{- end }}

{{/*
Compute the database URL.
Uses nexorious.databaseUrl if set; otherwise builds in-cluster URL.
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
