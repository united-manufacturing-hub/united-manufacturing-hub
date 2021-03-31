{{/*
Expand the name of the chart.
*/}}
{{- define "factorycube-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "factorycube-server.fullname" -}}
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
{{- define "factorycube-server.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "factorycube-server.labels.common" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
helm.sh/chart: {{ include "factorycube-server.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "factorycube-server.name" . }}
{{- end }}

{{/*
Labels for factoryinsight 
*/}}
{{- define "factorycube-server.labels.factoryinsight" -}}
app.kubernetes.io/name: {{ include "factorycube-server.name" . }}-factoryinsight
{{ include "factorycube-server.labels.common" . }}
{{- end }}

{{/*
Labels for mqtttopostgresql 
*/}}
{{- define "factorycube-server.labels.mqtttopostgresql" -}}
app.kubernetes.io/name: {{ include "factorycube-server.name" . }}-mqtttopostgresql
{{ include "factorycube-server.labels.common" . }}
{{- end }}

{{/*
Labels for nodered 
*/}}
{{- define "factorycube-server.labels.nodered" -}}
app.kubernetes.io/name: {{ include "factorycube-server.name" . }}-nodered
{{ include "factorycube-server.labels.common" . }}
{{- end }}

Labels for redis 
*/}}
{{- define "factorycube-server.labels.redis" -}}
app.kubernetes.io/name: {{ include "factorycube-server.name" . }}-redis
{{ include "factorycube-server.labels.common" . }}
{{- end }}

