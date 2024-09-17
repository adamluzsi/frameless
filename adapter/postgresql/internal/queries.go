package internal

import (
	"bytes"
	"text/template"
)

const queryEnsureSchemaMigrationsTableTmplText = `
CREATE TABLE IF NOT EXISTS "{{ .TableName }}" (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    namespace TEXT    NOT NULL,
	version   TEXT    NOT NULL,
	dirty     BOOLEAN NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS "{{ .TableName }}_namespace_version_idx"
	ON "{{ .TableName }}" ("namespace", "version");
`

type queryEnsureSchemaMigrationsTableTmplData struct {
	TableName string
}

var queryEnsureSchemaMigrationsTableTmpl = template.Must(template.New("create-schema-migration").Parse(queryEnsureSchemaMigrationsTableTmplText))

func QueryEnsureSchemaMigrationsTable(tableName string) (string, error) {
	var buf bytes.Buffer
	err := queryEnsureSchemaMigrationsTableTmpl.Execute(&buf, queryEnsureSchemaMigrationsTableTmplData{TableName: tableName})
	return buf.String(), err
}
