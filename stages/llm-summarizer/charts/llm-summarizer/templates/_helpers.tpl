{{- define "llm-summarizer.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "llm-summarizer.labels" -}}
app.kubernetes.io/name: {{ include "llm-summarizer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}
