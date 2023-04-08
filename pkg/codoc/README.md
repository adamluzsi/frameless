# CoDoc (experimental)

The name `codoc` stands for both "code documentation" and "cooperative documenting".
The idea behind codoc is to document the application with code which are part of the code paths. 
Traditionally documenting your software requires you to write a documentation,
including sequence diagrams and other helpful utilities to model your application, 
but these documents can quickly get outdated during active development.

Code documentation placed on top of the structures and functions can create great documentation for developers,
but they are rarely useful to product managers/owners and for other devs who are not familiar with your application's 
internal structuring, or with the language it was written in.

On top of that, it takes further effort to model systems, 
when an organisation uses distributed teams and microservices to implement the system/product.

This is where `codoc` can help. 
First it allows you do document your code with code. This way whenever you change your business logic,
the documentation follows can change as well.
Secondly, it allows you to build documents between multiple applications, 
using a distributed cooperative documenting approach.

Then, later when someone needs a birth overview, they can come to the UI, and take a look at the documentation.

`codoc` doesn't have to run during production use if you are uncomfortable
and run it either as part of your CI/CD pipeline (acceptance testing level)
or as part of the staging environment.

TODO:
- [ ] context based code sequence documented
- [ ] mermaid sequence UML exporter
- [ ] repository exporter to persist active code paths
- [ ] UI to make documentation accessible easily, and exportable
- [ ] option to include "interact with" options in a documentation
  - This can help with modelling the relationship between business logic and domain entities
