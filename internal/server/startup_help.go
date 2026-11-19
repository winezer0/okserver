package server

import (
	"io"
	"text/template"
)

func printStartupHelp(w io.Writer, values map[string]any) error {
	tpl, err := template.New("").Parse(`{{range .urls}}
web service: {{.}}
{{- end}}

server:
    {{.exec}} -c "{{.config}}" -s {{.listen}} -p "{{.dir}}"

cli download (SHA-256 verified, parallel ranges):
    {{.exec}} cli -c -parallel 4{{if .auth}} -auth "{{.auth}}"{{end}}{{if or .cert .key}} -ca "{{.ca}}"{{end}} -o "{{.example}}" "{{.addr}}/download/{{.example}}"
cli upload (serial chunks, resume with -c):
    {{.exec}} cli -c{{if .auth}} -auth "{{.auth}}"{{end}}{{if or .cert .key}} -ca "{{.ca}}"{{end}} -d "@{{.example}}" "{{.addr}}/upload/{{.example}}"
curl POST upload (client SHA-256 optional):
    curl --fail-with-body{{if .auth}} -u "{{.auth}}"{{end}} --data-binary "@{{.example}}" "{{.addr}}/upload/{{.example}}"
    curl --fail-with-body --data-binary "@{{.example}}" "{{.addr}}/upload/{{.example}}?sha256=<hex-sha256>"
wget POST upload:
    wget -O - --post-file="{{.example}}" "{{.addr}}/upload/{{.example}}"
upload offset:
    curl -I{{if .auth}} -u "{{.auth}}"{{end}} "{{.addr}}/upload/{{.example}}"
    Continue with curl -C <Upload-Offset> -T "{{.example}}" "{{.addr}}/upload/{{.example}}"
    Serial chunks use PUT with Content-Range: bytes <start>-<end>/<total>.
download SHA-256 (X-File-SHA256 response header):
    curl -I{{if .auth}} -u "{{.auth}}"{{end}} "{{.addr}}/download/{{.example}}"

`)
	if err != nil {
		return err
	}
	return tpl.Execute(w, values)
}
