# MariaDB Adapter Package

This package provides a MariaDB adapter for the Frameless.
It allows you to interact with MariaDB databases using a standardized interface.

## **Features**

* Connection management: Establish and manage connections to MariaDB databases.
* CRUD operations: Perform create, read, update, and delete operations on MariaDB tables.
* Transaction support: Use transactions to ensure atomicity and consistency of database operations.
* Migration support: Use migrations to manage schema changes and versioning of your database.
* use MariaDB as caching backend

**Getting Started**

To use this package, you need to install it using Go's package manager:
```bash
go get go.llib.dev/frameless/adapter/mariadb
```

Then, import the package in your Go program:
```go
import "go.llib.dev/frameless/adapter/mariadb"
```

Create a connection to a MariaDB database using the `Connect` function:
```go
conn, err := mariadb.Connect("user:password@tcp(localhost:3306)/database")
```

Use the `Repository` type to perform CRUD operations on a table:
```go
repo := mariadb.Repository[Entity, ID]{
    Connection: conn,
    Mapping:    EntityMapping(),
}

// Create an entity
entity := Entity{Name: "John Doe"}
err := repo.Create(context.Background(), &entity)
```

**License**

This package is licensed under the MIT License.