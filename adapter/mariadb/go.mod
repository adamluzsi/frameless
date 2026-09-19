module go.llib.dev/frameless/adapter/mariadb

go 1.25

require (
	github.com/go-sql-driver/mysql v1.10.1
	go.llib.dev/frameless v0.338.0
	go.llib.dev/testcase v0.194.2
)

replace go.llib.dev/frameless => ../..

require filippo.io/edwards25519 v1.2.0 // indirect
