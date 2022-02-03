{{/*
Expand the name of the chart.
*/}}
{{- define "united-manufacturing-hub.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "united-manufacturing-hub.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Release.Name .Values.nameOverride }}
{{- printf "%s" $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "united-manufacturing-hub.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "united-manufacturing-hub.labels.common" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
helm.sh/chart: {{ include "united-manufacturing-hub.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "united-manufacturing-hub.name" . }}
{{- end }}

{{/*
Labels for barcodereader
*/}}
{{- define "united-manufacturing-hub.labels.barcodereader" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-barcodereader
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for mqtt-bridge
*/}}
{{- define "united-manufacturing-hub.labels.mqttbridge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqttbridge
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for factoryinsight
*/}}
{{- define "united-manufacturing-hub.labels.factoryinsight" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-factoryinsight
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for mqtttopostgresql
*/}}
{{- define "united-manufacturing-hub.labels.mqtttopostgresql" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqtttopostgresql
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for mqtttoblob
*/}}
{{- define "united-manufacturing-hub.labels.mqtttoblob" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqtttoblob
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for nodered
*/}}
{{- define "united-manufacturing-hub.labels.nodered" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-nodered
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for redis
*/}}
{{- define "united-manufacturing-hub.labels.redis" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-redis
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for timescaledb
*/}}
{{- define "united-manufacturing-hub.labels.timescaledb" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-timescaledb
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for grafanaproxy
*/}}
{{- define "united-manufacturing-hub.labels.grafanaproxy" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-grafanaproxy
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}


{{/*
Labels for factoryinput
*/}}
{{- define "united-manufacturing-hub.labels.factoryinput" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-factoryinput
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for kafkatoblob
*/}}
{{- define "united-manufacturing-hub.labels.kafkatoblob" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkatoblob
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for mqttkafkabridge
*/}}
{{- define "united-manufacturing-hub.labels.mqttkafkabridge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqttkafkabridge
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}


{{/*
Labels for sensorconnect
*/}}
{{- define "united-manufacturing-hub.labels.sensorconnect" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-sensorconnect
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for cameraconnect
*/}}
{{- define "united-manufacturing-hub.labels.cameraconnect" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-cameraconnect
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}

{{/*
Labels for vernemq
*/}}
{{- define "united-manufacturing-hub.labels.vernemq" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-vernemq
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}


{{/*
Labels for emqxedge
*/}}
{{- define "united-manufacturing-hub.labels.emqxedge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-emqxedge
{{ include "united-manufacturing-hub.labels.common" . }}
{{- end }}


{{/*
Create the name of the service account to use
*/}}
{{- define "united-manufacturing-hub.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "united-manufacturing-hub.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
