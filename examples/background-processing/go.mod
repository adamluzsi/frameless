module background-processing

go 1.26.0

require (
	go.llib.dev/frameless v0.340.1
	go.llib.dev/frameless/adapter/postgresql v0.22.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	go.llib.dev/testcase v0.195.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace (
	go.llib.dev/frameless => ../..
	go.llib.dev/frameless/adapter/postgresql => ../../adapter/postgresql
)
