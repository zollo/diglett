{{- define "diglett.name" -}}
diglett
{{- end -}}

{{- define "diglett.labels" -}}
app.kubernetes.io/name: {{ include "diglett.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "diglett.selectorLabels" -}}
app.kubernetes.io/name: {{ include "diglett.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
