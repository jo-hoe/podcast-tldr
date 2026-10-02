{{- define "rss-audio-downloader.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "rss-audio-downloader.labels" -}}
app.kubernetes.io/name: {{ include "rss-audio-downloader.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}
