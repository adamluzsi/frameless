package pgstorage

import (
	"database/sql"
	"github.com/adamluzsi/frameless"
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

func (pg *PG) Close() error {
	return pg.Close()
}

func (pg *PG) Exec(query frameless.Query) frameless.Iterator {
	//switch data := query.(type) {
	//
	//default:
	//	return iterators.NewError(queryerrors.ErrNotImplemented)
	//}
	return nil
}
