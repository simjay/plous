{{- define "playground.labels" -}}
app.kubernetes.io/part-of: plous-playground
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}
{{- define "playground.resources" -}}
requests:
  cpu: 10m
  memory: 16Mi
limits:
  cpu: 200m
  memory: 64Mi
{{- end -}}
