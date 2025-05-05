{{/* Common name definitions */}}
{{- define "wireguard.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fullname generator */}}
{{- define "wireguard.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "wireguard.selectorLabels" -}}
app.kubernetes.io/name: {{ include "wireguard.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
{{- define "wstunnel.selectorLabels" -}}
app.kubernetes.io/name: {{ include "wireguard.name" . }}-wstunnel
app.kubernetes.io/instance: {{ .Release.Name }}-wstunnel
{{- end -}}