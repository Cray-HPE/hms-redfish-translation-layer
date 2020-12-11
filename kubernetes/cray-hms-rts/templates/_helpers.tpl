{{/*
Add helper methods here for your chart
*/}}

{{- define "cray-hms-redfish-translation-layer.image-prefix" -}}
{{ $base := index . "cray-service" }}
{{- if $base.imagesHost -}}
{{- printf "%s/" $base.imagesHost -}}
{{- else -}}
{{- printf "" -}}
{{- end -}}
{{- end -}}

{{/*
Helper function to get the proper image tag
*/}}
{{- define "cray-hms-redfish-translation-layer.imageTag" -}}
{{- default "latest" .Chart.AppVersion -}}
{{- end -}}
