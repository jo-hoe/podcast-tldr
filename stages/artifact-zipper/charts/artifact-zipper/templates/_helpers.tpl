{{- define "artifact-zipper.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "artifact-zipper.labels" -}}
app.kubernetes.io/name: {{ include "artifact-zipper.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}
