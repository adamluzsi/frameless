Data Transfer Objects (DTOs) Package
====================================

This package provides an easy-to-use and maintainable way
to create DTOs and map between entities and their corresponding DTOs in Go applications.
The package simplifies the process of creating DTOs,
while ensuring type safety and reducing the potential for errors.

Features
--------

* Simplified creation of Data Transfer Objects (DTOs)
* Mapping between entities and DTOs using custom mapping functions
* Automatic handling of nested entities
* Type-safe DTO creation and mapping
* Support for multiple DTO mappings for different serialization formats, such as JSON or YAML
* Ability to register multiple DTOs for a single entity

Getting Started
---------------

To get started, import the package into your Go application:

```go
import "go.llib.dev/frameless/pkg/dtokit"
```

### Registering Mapping

The `dtokit.Register` function is used to register the mappings between entities and their respective dtokit.
In this example, we're using a `JSONMapping` object to contain the mapping configurations:

```go
var JSONMapping dtokit.M
var _ = dtokit.Register[Ent, EntDTO](&JSONMapping, EntMapping{})
var _ = dtokit.Register[NestedEnt, NestedEntDTO](&JSONMapping, NestedEntMapping{})
```

### Defining Mappings

The mappings are defined using custom mapping functions for each entity-DTO pair:

```go
package exampe

type Ent struct {
	V int
}

type EntDTO struct {
	V string `json:"v"`
}

type EntMapping struct{}

func (EntMapping) ToDTO(_ *dtokit.M, ent Ent) (EntDTO, error) {
	return EntDTO{V: strconv.Itoa(ent.V)}, nil
}

func (EntMapping) ToEnt(m *dtokit.M, dto EntDTO) (Ent, error) {
	v, err := strconv.Atoi(dto.V)
	if err != nil {
		return Ent{}, err
	}
	return Ent{V: v}, nil
}
```

### Using Different Mapping Registries Per Serialization Formats

The package allows you to register different mappings for different serialization formats,
such as JSON or YAML, or for different use cases.
This makes it easy to maintain and update your mappings over time even if got a larger number of DTO models.

Here is an example of how to register a JSON-specific mapping and a YAML-specific mapping:

```go
var JSONMapping dtokit.M
_ = dtokit.Register[Ent, EntJSONDTO](&JSONMapping, EntJSONMapping{})

var YAMLMapping dtokit.M
_ = dtokit.Register[Ent, EntYAMLDTO](&YAMLMapping, EntYAMLMapping{})
```

You can utilise distinct mapping registers as various mapping versions, simplifying the versioning process for your endpoint.

### Using a Single Mapping Registry For Multiple Serialization Formats

The package also allows you to use a single mapping registry (`dtokit.M`) to manage all your DTO mappings.
This can be useful when you have moderate number of DTOs, and want to keep things simple.

Here is an example:

```go
var DTOMappings dtokit.M
var _ = dtokit.Register[Ent, EntJSONDTO](&DTOMappings, EntJSONMapping{})
var _ = dtokit.Register[Ent, EntYAMLDTO](&DTOMappings, EntYAMLMapping{})
```

### Mapping Entities to DTOs & Marshalling

The `dtokit.Map` function is used to convert an entity object into a target DTO object.

Once the DTO has been created, it can be marshaled into a the target format.
For Example in case of `json` you can use Go's built-in `json.Marshal` function:

```go
package exampe

func fn() {
	var v = NestedEnt{
		ID: "42",
		Ent: Ent{
			V: 42,
		},
	}

	dto, err := dtokit.Map[NestedEntDTO](&JSONMapping, v)
	if err != nil { // handle err
		return
	}

	data, err := json.Marshal(dto)
	if err != nil { // handle error
		return
	}
	/*
		{
			"id": "42",
			"ent": {
				"v": "42"
			}
		}
	*/
}

```
