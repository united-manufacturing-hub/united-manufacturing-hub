
{{/*
Expand the name of the chart.
*/}}
{{- define "factorycube-core.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "factorycube-core.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "factorycube-core.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "factorycube-core.labels.common" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
helm.sh/chart: {{ include "factorycube-core.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "factorycube-core.name" . }}
{{- end }}

{{/*
Labels for nodered
*/}}
{{- define "factorycube-core.labels.nodered" -}}
app.kubernetes.io/name: {{ include "factorycube-core.name" . }}-nodered
{{ include "factorycube-core.labels.common" . }}
{{- end }}

{{/*
Labels for sensorconnect
*/}}
{{- define "factorycube-core.labels.sensorconnect" -}}
app.kubernetes.io/name: {{ include "factorycube-core.name" . }}-sensorconnect
{{ include "factorycube-core.labels.common" . }}
{{- end }}

{{/*
Labels for vernemq
*/}}
{{- define "factorycube-core.labels.vernemq" -}}
app.kubernetes.io/name: {{ include "factorycube-core.name" . }}-vernemq
{{ include "factorycube-core.labels.common" . }}
{{- end }}


{{/*
Create the name of the service account to use
*/}}
{{- define "factorycube-core.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "factorycube-core.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
