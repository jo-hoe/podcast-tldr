{{- define "git-archive-backup.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "git-archive-backup.labels" -}}
app.kubernetes.io/name: {{ include "git-archive-backup.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}
