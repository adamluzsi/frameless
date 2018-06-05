# Frameless

What this package do ?

## Package frameless aims to create convention to build software from ground instead of upside down.

 But what does this in in practice? This package makes you be less productive in speed for short term! :)

 The goal of the package is the learning and practicing [The Clean Architecture](https://8thlight.com/blog/uncle-bob/2012/08/13/the-clean-architecture.html).
 By doing so you will create web/cli/{{ .ChannelName }} applications that actually external channel and framework independent.

 By working disciplined and separate the scope of your code and enforcing [law of demeter](https://en.wikipedia.org/wiki/Law_of_Demeter) on your architecture,
 by my you probably end up with something that is boring in term of code, not have fancy.

 The results will be something like

  * boring code
  * separation between
    * external interface
    * external resource
    * template layer
    * presentation & serialization
    * application control logic
      * use cases
      * controllers
    * Business entities


 Yes but how will this help me to achieve this

 Basically because because the overwhelming possibility to what technology use for a project,
 sometimes these days reverse the traditional development from ground up to upside down way.
 So instead of starting to create pure Business rules and business core entities,
 the developer tricked into start working from End2End through external interface point of view.
 Such example is when a developer creates an application through testing (sometimes manually) from the webpage point of view,
 or like "If I click this html button on the side bar, there should be a new record in the database with xy".
 While it has faster impact in look, usually the business rules rarely created independently from the framework and external resources such as the db.

 While following the ideologies presented in the project, you will create applications that will be build from ground.
 You will basically create the pure business entities, than business "use cases"/rules with them,
 and as a final move, you choose what should be the external interface (cli/mq/http/{{.Channel}}).


 Business Entity

 Entities encapsulate the most general and high-level rules of the application.
 	"An entity can be an object with methods, or it can be a set of data structures and functions"
 	Robert Martin

 I tried different structures during my research, and in the end the most maintainable one was an
 interface that describe the high level behavior of an entity and a shared runable specification.
 This shared specification used to test against the underling implementations.
 They behavior are the least likely to change when something external changes.
 	For example, you would not expect these objects to be affected by how they used and what behavior they implement
 	when a change to page navigation, or security happen.

 In other languages my preference is RSpec, Jasmine or Oleaster for creating shared specifications but it is up to you what you prefer the most.
 Also the Business Entity must not give back any value that is implementation specific!
 	for example when you call a method/function on this entity, you should not receive sql rows object

 Example:

		type User interface{
			frameless.Persistable

			Name() string
			Email() string
			Biography() string
		}


 Handling relationship between business entities

 Relations between business entities should be implemented by function relations and controller should not know how to query the underling structure.
 For example:

		type User interface{
			frameless.Persistable
			Teams() frameless.HasManyRelationship
		}



 Controllers

 	The controller responds to the user input and performs interactions on the data model objects.
 	The controller receives the input, optionally validates it and then passes the input to the model.

 To me when I played with the Separate Controller/UseCase layers in a production app, I felt the extra layer just was cumbersome.
 Therefore the "controller" concept here is a little different from traditional controllers but not different what most of the time a controller do in real world applications.
 Implementing business use cases, validation or you name it. From the simples case such as listing users, to the extend of executing heavy calculations on user behalf.
 The classical controller that provides interface between an external interface and a business logic implementation is something that must be implemented in the external interface integration layer itself.
 I removed this extra layer to make controller scope only to control the execution of a specific business use case based on generic inputs such as presenter and request.
 Also there is a return error which represent the unexpected and not recoverable errors that should notified back to the caller to teardown execution nicely.

  The software in this layer contains application specific business rules.
  It encapsulates and implements all of the use cases of the system.
  These use cases orchestrate the flow of data to and from the entities,
  and direct those entities to use their enterprise wide business rules to achieve the goals of the use case.

  We do not expect changes in this layer to affect the entities.
  We also do not expect this layer to be affected by changes to externalities such as the database,
  the UI, or any of the common frameworks.
  This layer is isolated from such concerns.

  We do, however, expect that changes to the operation of the application will affect the use-cases and therefore the software in this layer.
  If the details of a use-case change, then some code in this layer will certainly be affected.

  Robert Martin


# "Yes, but...

## ... it looks to me you want to force interfaces to my duck type language"

Of course not, for example, in ruby you not do compile based interface contracts but explicit specification based contracts.
Basically you define your business entities in a testing specification that could be included in every other test where it will be dependency.
You can also make it work with "in memory" implementation for the sake of tests, which result in a fast testing suite execution.
