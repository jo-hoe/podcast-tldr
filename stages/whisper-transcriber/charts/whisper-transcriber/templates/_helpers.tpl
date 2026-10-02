{{- define "whisper-transcriber.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "whisper-transcriber.labels" -}}
app.kubernetes.io/name: {{ include "whisper-transcriber.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}
