{{/*
Expand the name of the chart.
*/}}
{{- define "factorycube-unified.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "factorycube-unified.fullname" -}}
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
{{- define "factorycube-unified.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "factorycube-unified.labels.common" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
helm.sh/chart: {{ include "factorycube-unified.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "factorycube-unified.name" . }}
{{- end }}

{{/*
Labels for barcodereader
*/}}
{{- define "factorycube-unified.labels.barcodereader" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-barcodereader
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for mqtt-bridge
*/}}
{{- define "factorycube-unified.labels.mqttbridge" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-mqttbridge
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for factoryinsight
*/}}
{{- define "factorycube-unified.labels.factoryinsight" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-factoryinsight
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for mqtttopostgresql
*/}}
{{- define "factorycube-unified.labels.mqtttopostgresql" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-mqtttopostgresql
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for mqtttoblob
*/}}
{{- define "factorycube-unified.labels.mqtttoblob" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-mqtttoblob
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for nodered
*/}}
{{- define "factorycube-unified.labels.nodered" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-nodered
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for redis
*/}}
{{- define "factorycube-unified.labels.redis" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-redis
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for timescaledb
*/}}
{{- define "factorycube-unified.labels.timescaledb" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-timescaledb
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for grafanaproxy
*/}}
{{- define "factorycube-unified.labels.grafanaproxy" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-grafanaproxy
{{ include "factorycube-unified.labels.common" . }}
{{- end }}


{{/*
Labels for factoryinput
*/}}
{{- define "factorycube-unified.labels.factoryinput" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-factoryinput
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for kafkatoblob
*/}}
{{- define "factorycube-unified.labels.kafkatoblob" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-kafkatoblob
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for mqttkafkabridge
*/}}
{{- define "factorycube-unified.labels.mqttkafkabridge" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-mqttkafkabridge
{{ include "factorycube-unified.labels.common" . }}
{{- end }}


{{/*
Labels for sensorconnect
*/}}
{{- define "factorycube-unified.labels.sensorconnect" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-sensorconnect
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for cameraconnect
*/}}
{{- define "factorycube-unified.labels.cameraconnect" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-cameraconnect
{{ include "factorycube-unified.labels.common" . }}
{{- end }}

{{/*
Labels for vernemq
*/}}
{{- define "factorycube-unified.labels.vernemq" -}}
app.kubernetes.io/name: {{ include "factorycube-unified.name" . }}-vernemq
{{ include "factorycube-unified.labels.common" . }}
{{- end }}


{{/*
Labels for emqxedge
*/}}
{{- define "factorycube-edge.labels.emqxedge" -}}
app.kubernetes.io/name: {{ include "factorycube-edge.name" . }}-emqxedge
{{ include "factorycube-edge.labels.common" . }}
{{- end }}


{{/*
Create the name of the service account to use
*/}}
{{- define "factorycube-unified.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "factorycube-unified.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
