{{/*
Common labels for all resources.
*/}}
{{- define "ai-gauntlet.labels" -}}
app.kubernetes.io/name: ai-gauntlet
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Per-component resource names. Using Release.Name keeps the same name we used
under the kustomize layout when installed as `helm install ai-gauntlet ...`.
*/}}
{{- define "ai-gauntlet.backend.fullname" -}}
{{ .Release.Name }}-backend
{{- end -}}

{{- define "ai-gauntlet.frontend.fullname" -}}
{{ .Release.Name }}-frontend
{{- end -}}

{{- define "ai-gauntlet.postgres.fullname" -}}
{{ .Release.Name }}-postgres
{{- end -}}

{{/*
DATABASE_URL pieced together from the postgres service + Secret values.
The $(VAR) syntax is resolved by the kubelet from the container's env, so
POSTGRES_USER / POSTGRES_PASSWORD / POSTGRES_DB must also be loaded into env
on the same container (see envFrom on the backend Deployment).
*/}}
{{- define "ai-gauntlet.databaseUrl" -}}
postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@{{ include "ai-gauntlet.postgres.fullname" . }}:5432/$(POSTGRES_DB)?sslmode=disable
{{- end -}}
