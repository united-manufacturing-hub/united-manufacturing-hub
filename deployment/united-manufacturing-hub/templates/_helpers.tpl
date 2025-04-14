{*
 Copyright 2025 UMH Systems GmbH

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*}

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
Common labels. These can be used as metadata by all resources in the chart.
DO NOT USE FOR SELECTOR LABELS.
*/}}
{{- define "united-manufacturing-hub.labels.common" -}}
{{ include "united-manufacturing-hub.matchLabels" . }}
helm.sh/chart: {{ include "united-manufacturing-hub.chart" . }}
{{- end }}

{{/*
Match labels. These can be used as selector labels in .spec.selector.matchLabels and are meant to be immutable
*/}}
{{- define "united-manufacturing-hub.matchLabels" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ include "united-manufacturing-hub.name" . }}
{{- end -}}


{{/*
Labels for barcodereader
*/}}
{{- define "united-manufacturing-hub.labels.barcodereader" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-barcodereader
{{- end }}

{{/*
Labels for mqtt-bridge
*/}}
{{- define "united-manufacturing-hub.labels.mqttbridge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqttbridge
{{- end }}

{{/*
Labels for factoryinsight
*/}}
{{- define "united-manufacturing-hub.labels.factoryinsight" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-factoryinsight
{{- end }}

{{/*
Labels for mqtttopostgresql
*/}}
{{- define "united-manufacturing-hub.labels.mqtttopostgresql" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqtttopostgresql
{{- end }}

{{/*
Labels for mqtttoblob
*/}}
{{- define "united-manufacturing-hub.labels.mqtttoblob" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqtttoblob
{{- end }}

{{/*
Labels for nodered
*/}}
{{- define "united-manufacturing-hub.labels.nodered" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-nodered
{{- end }}

{{/*
Labels for redis
*/}}
{{- define "united-manufacturing-hub.labels.redis" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-redis
{{- end }}

{{/*
Labels for timescaledb
*/}}
{{- define "united-manufacturing-hub.labels.timescaledb" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-timescaledb
{{- end }}

{{/*
Labels for packmlmqttsimulator
*/}}
{{- define "united-manufacturing-hub.labels.packmlmqttsimulator" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-packmlmqttsimulator
{{- end }}


{{/*
Labels for grafanaproxy
*/}}
{{- define "united-manufacturing-hub.labels.grafanaproxy" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-grafanaproxy
{{- end }}


{{/*
Labels for factoryinput
*/}}
{{- define "united-manufacturing-hub.labels.factoryinput" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-factoryinput
{{- end }}

{{/*
Labels for kafkatoblob
*/}}
{{- define "united-manufacturing-hub.labels.kafkatoblob" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkatoblob
{{- end }}

{{/*
Labels for mqttkafkabridge
*/}}
{{- define "united-manufacturing-hub.labels.mqttkafkabridge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-mqttkafkabridge
{{- end }}


{{/*
Labels for sensorconnect
*/}}
{{- define "united-manufacturing-hub.labels.sensorconnect" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-sensorconnect
{{- end }}

{{/*
Labels for cameraconnect
*/}}
{{- define "united-manufacturing-hub.labels.cameraconnect" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-cameraconnect
{{- end }}

{{/*
Labels for iotsensorsmqtt
*/}}
{{- define "united-manufacturing-hub.labels.iotsensorsmqtt" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-iotsensorsmqtt
app.kubernetes.io/component: "iotsensorsmqtt"
{{- end }}


{{/*
Labels for opcuasimulator
*/}}
{{- define "united-manufacturing-hub.labels.opcuasimulator" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-opcuasimulator
app.kubernetes.io/component: "opcuasimulator"
{{- end }}

{{/*
Labels for opcsimv2
*/}}
{{- define "united-manufacturing-hub.labels.opcsimv2" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-opcsimv2
app.kubernetes.io/component: "opcsimv2"
{{- end }}

{{/*
Labels for modbussimulator
*/}}
{{- define "united-manufacturing-hub.labels.modbussimulator" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-modbussimulator
app.kubernetes.io/component: "modbussimulator"
{{- end }}

{{/*
Labels for hivemqce
*/}}
{{- define "united-manufacturing-hub.labels.hivemqce" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-hivemqce
app.kubernetes.io/component: "hivemqce"
{{- end }}

{{/*
Labels for emqxedge
*/}}
{{- define "united-manufacturing-hub.labels.emqxedge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-emqxedge
{{- end }}


{{/*
Labels for kafkatopostgresql
*/}}
{{- define "united-manufacturing-hub.labels.kafkatopostgresql" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkatopostgresql
{{- end }}

{{/*
Labels for kafkastatedetector
*/}}
{{- define "united-manufacturing-hub.labels.kafkastatedetector" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkastatedetector
{{- end }}

{{/*
Labels for kafkabridge
*/}}
{{- define "united-manufacturing-hub.labels.kafkabridge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkabridge
{{- end }}

{{/*
Labels for kafkadebug
*/}}
{{- define "united-manufacturing-hub.labels.kafkadebug" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkadebug
{{- end }}

{{/*
Labels for kafkainit
*/}}
{{- define "united-manufacturing-hub.labels.kafkainit" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkainit
{{- end }}

{{/*
Labels for kafka
*/}}
{{- define "united-manufacturing-hub.labels.kafka" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafka
{{- end }}

{{/*
Labels for kowl
*/}}
{{- define "united-manufacturing-hub.labels.kowl" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kowl
{{- end }}

{{/*
Labels for tulip-connector
*/}}
{{- define "united-manufacturing-hub.labels.tulip-connector" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-tulip-connector
{{- end }}

{{/*
Labels for metrics
*/}}
{{- define "united-manufacturing-hub.labels.metrics-cron" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-metrics-cron
{{- end }}

{{- define "united-manufacturing-hub.labels.metrics-install" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-metrics-install
{{- end }}

{{- define "united-manufacturing-hub.labels.metrics-upgrade" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-metrics-upgrade
{{- end }}

{{- define "united-manufacturing-hub.labels.metrics-delete" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-metrics-delete
{{- end }}

{{- define "united-manufacturing-hub.labels.metrics-rollback" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-metrics-rollback
{{- end }}

{{/*
Labels for databridge
*/}}
{{- define "united-manufacturing-hub.labels.databridge" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-databridge
{{- end }}

{{/*
Labels for kafkatopostgresqlv2
*/}}
{{- define "united-manufacturing-hub.labels.kafkatopostgresqlv2" -}}
app.kubernetes.io/name: {{ include "united-manufacturing-hub.name" . }}-kafkatopostgresqlv2
{{- end }}


{{/*
Create the name of the service account to use
*/}}
{{- define "united-manufacturing-hub.serviceAccountName" -}}
{{- include "united-manufacturing-hub.fullname" . }}
{{- end }}

{{/*
Define HiveMQ version (2024.1)
*/}}
{{- define "united-manufacturing-hub.hivemq.version" -}}
2024.1
{{- end }}

{{/*
Define Postgresq passwords
*/}}
{{- define "united-manufacturing-hub.postgresql.factoryinsight.password" -}}
changeme
{{- end }}

{{- define "united-manufacturing-hub.postgresql.kafkatopostgresqlv2.password" -}}
changemetoo
{{- end }}

{{- define "united-manufacturing-hub.postgresql.grafanareader.password" -}}
changeme
{{- end }}
