{{/*
Expand the name of the chart.
*/}}
{{- define "diglett.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name, truncated to the 63-character DNS label limit.
*/}}
{{- define "diglett.fullname" -}}
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

{{- define "diglett.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "diglett.labels" -}}
helm.sh/chart: {{ include "diglett.chart" . }}
{{ include "diglett.selectorLabels" . }}
app.kubernetes.io/version: {{ include "diglett.imageTag" . | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "diglett.selectorLabels" -}}
app.kubernetes.io/name: {{ include "diglett.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "diglett.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "diglett.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image tag: explicit value, else the chart appVersion, else "latest".
*/}}
{{- define "diglett.imageTag" -}}
{{- .Values.image.tag | default .Chart.AppVersion | default "latest" }}
{{- end }}

{{- define "diglett.image" -}}
{{- printf "%s:%s" .Values.image.repository (include "diglett.imageTag" .) }}
{{- end }}
