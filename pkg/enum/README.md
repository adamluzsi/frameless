# enum

A Go package for working with enumerated types (enums).
It provides easy validation, registration, and reflection of enums,
including support for struct validation, and slice-element enums validation.

## Table of Contents

- [enum](#enum)
	- [Table of Contents](#table-of-contents)
	- [Overview](#overview)
	- [Installation](#installation)
	- [Usage](#usage)

## Overview

The `enum` package helps you define, validate, and work with enumerated types in Go.

It supports various types like:
- primitive types like `int`, `string`, `float`, `bool`
- sub types of any primitive type
- Slice element enumerable validation
- struct tag support to enable seamless integration into your structs.

This package allows you to:

- Validate a value against its enumerators.
- Nested validate the fields of a struct against predefined enum values.
- Register and unregister enum values dynamically.
- Use enumerators from reflection.

## Installation

To install the package, use the following command:

```bash
go get go.llib.dev/frameless/pkg/enum
```

## Usage

You can either use the `enum.Register` or set the `enum` tag on a given struct field's tag.

If you use `enum.Register` for a given type, then any time the given type is validated,
the registered enumerators will be used for comparison.

You can use various formats to define an enumerator list for a given type in a struct field:

- with one of thedefault seperators:
  - space seperated list: `enum:"foo bar baz"`
  - comma seperated list: `enum:"foo,bar,baz"`
  - they support optional spacing around the values like `enum:"foo, bar, baz"
- with an explicit seperator
  - is the last character is a special character, then it will be used as the seperator value
    - here is an example where we want comma and space as valid enumeration values, thus we choose "|" as our preferred symbol to seperate the enumerator values:
	  `enum:",| |&|"
	- if we choose our own seperator symbol, then it will respect the values between the seperators as is, and won't apply any space trimming.

**basic-example:**

```go
package main

import (
	"fmt"

	"go.llib.dev/frameless/pkg/enum"
)

type ExampleStruct struct {
	V  string `enum:"foo bar baz"`
	VS []int  `enum:"2,4,6,8"`
}

func main() {
	err := enum.Validate(ExampleStruct{V: "A"})
	if err != nil {
		fmt.Println("Validation failed:", err)
		return
	}
}
```

```go
type MyStruct struct {
	V string `enum:"A B C"`
}

_ = enum.ValidateStruct(MyStruct{V: "A"}) // no error
_ = enum.ValidateStruct(MyStruct{V: "D"}) // has error
```

**creating a custom enum type:**

```go
package main

import (
	"fmt"

	"go.llib.dev/frameless/pkg/enum"
)

type MyEnumType string

const (
	Option1 MyEnumType = "Option1"
	Option2 MyEnumType = "Option2"
	Option3 MyEnumType = "Option3"
)

var _ = enum.Register[MyEnumType](Option1, Option2, Option3)

func main() {
	err := enum.Validate(Option1) // Valid
	if err != nil {
		fmt.Println("Error:", err)
	}

	err = enum.Validate(MyEnumType("InvalidOption")) // Invalid
	if err != nil {
		fmt.Println("Error:", err)
	}
}
```




Validation

The Validate function can be used to check if a value is a valid enum. The function supports both primitive types (like int, string, bool) and custom enum types.

Struct Validation

The ValidateStruct function validates struct fields tagged with the enum tag. This allows struct-level enum validation:

type MyStruct struct {
    Status string `enum:"Active|Inactive|Pending"`
}

err := enum.ValidateStruct(MyStruct{Status: "Active"}) // No error

Registering Enums

You can register custom enum values for a specific type, which allows the type to be validated dynamically:

type MyEnum string

const (
	First MyEnum = "First"
	Second MyEnum = "Second"
	Third MyEnum = "Third"
)

enum.Register[MyEnum](First, Second, Third)

You can unregister enum values by calling the return value from the Register function:

unregister := enum.Register[MyEnum](First, Second, Third)
defer unregister()

Reflecting Enums

The package also provides reflection-based functions for retrieving enum values:

Reflect Values

enum.ReflectValues(reflect.TypeOf((*MyEnum)(nil)).Elem())

This will return a slice of reflect.Value containing all registered enum values.

Reflect Struct Field Enums

field, ok := reflect.TypeOf(MyStruct{}).FieldByName("Status")
values, err := enum.ReflectValuesOfStructField(field)

This function returns all enum values defined in a struct field’s tag.

Functions

Here are the core functions provided by the package:

ValidateStruct

Validates a struct to ensure all fields that are tagged with enum have valid enum values.

Validate

Validates a single value against registered enum values.

Register

Registers enum values for a specific type, allowing runtime validation.

Values

Returns a slice of registered enum values for a type.

ReflectValues

Returns a slice of enum values for a given type using reflection.

ReflectValuesOfStructField

Returns a slice of enum values for a struct field defined with the enum tag.

Testing

The package includes various test cases to ensure correct behaviour across different use cases:
	•	Validating different enum types.
	•	Registering and unregistering enums dynamically.
	•	Handling edge cases, such as invalid enum values and unsupported types.
	•	Reflecting over struct fields to retrieve enum values.

To run the tests, use the following command:

go test

License

This project is licensed under the MIT License - see the LICENSE file for details.

This `README.md` provides a clear and concise overview of how the package works, including examples of validation, registration, and reflection. It also includes sections for installation, usage, and testing to make it easier for others to understand and start using the package.