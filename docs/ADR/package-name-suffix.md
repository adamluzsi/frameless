# Using the "kit" suffix instead of "util" in package names

## Context

When a pacakge in frameless meant to support a core functionality, until now, the stdlib convention was used,
and the package name received name of the target topic plus a "util" suffix. For example: `errorutil`, `pathutil`

However, when using such suffixes, there is a risk of name collision with other packages, including those in the
standard library.

The frameless project has a package named "httputil", which overlap with the "httputil" package in the standard library.
This could cause a developer to import the wrong package unintentionally, when they use code formatting tools.

## Decision

To avoid conflicts with package names in the standard library,
we have decided to use the "kit" suffix instead of "util" in our package names.
This suffix is commonly used in the Go community as an alternative to "util".

By using the "kit" suffix, we can avoid name collisions with the standard library and other packages,
and make it easier to import the correct dependencies from code editors with Go language support.

Additionally, the "kit" suffix is also descriptive and expressive way to indicate
that a package contains a collection of related tools, utilities, or components.

## Consequences

This decision will require us to rename some of our existing packages that currently use the "util" suffix.
For example, we will need to rename "httputil" to "httpkit". 
This change will require updating imports in any code that uses these packages.

While this change may cause some initial inconvenience,
we believe that it will lead to better code organization and maintainability in the long run.

It will also help us avoid conflicts with other packages
and reduce the risk of confusion and errors caused by package name collisions.
