package pgstroage

import (
	"database/sql"
	_ "github.com/lib/pq"
)

func NewPG(dataSourceName string) (*PG, error) {
	db, err := sql.Open("postgres", dataSourceName)

	if err != nil {
		return nil, err
	}

	return &PG{DB: db}, nil
}

type PG struct {
	DB *sql.DB
}
