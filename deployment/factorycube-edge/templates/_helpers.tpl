
{{/*
Expand the name of the chart.
*/}}
{{- define "factorycube-edge.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "factorycube-edge.fullname" -}}
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
{{- define "factorycube-edge.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "factorycube-edge.labels.common" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
helm.sh/chart: {{ include "factorycube-edge.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "factorycube-edge.name" . }}
{{- end }}

{{/*
Labels for barcodereader 
*/}}
{{- define "factorycube-edge.labels.barcodereader" -}}
app.kubernetes.io/name: {{ include "factorycube-edge.name" . }}-barcodereader
{{ include "factorycube-edge.labels.common" . }}
{{- end }}

{{/*
Labels for mqtt-bridge 
*/}}
{{- define "factorycube-edge.labels.mqttbridge" -}}
app.kubernetes.io/name: {{ include "factorycube-edge.name" . }}-mqttbridge
{{ include "factorycube-edge.labels.common" . }}
{{- end }}

{{/*
Labels for nodered
*/}}
{{- define "factorycube-edge.labels.nodered" -}}
app.kubernetes.io/name: {{ include "factorycube-edge.name" . }}-nodered
{{ include "factorycube-edge.labels.common" . }}
{{- end }}

{{/*
Labels for sensorconnect
*/}}
{{- define "factorycube-edge.labels.sensorconnect" -}}
app.kubernetes.io/name: {{ include "factorycube-edge.name" . }}-sensorconnect
{{ include "factorycube-edge.labels.common" . }}
{{- end }}

{{/*
Labels for vernemq
*/}}
{{- define "factorycube-edge.labels.vernemq" -}}
app.kubernetes.io/name: {{ include "factorycube-edge.name" . }}-vernemq
{{ include "factorycube-edge.labels.common" . }}
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
{{- define "factorycube-edge.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "factorycube-edge.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
