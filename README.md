# Frameless

What this package do ?
Well in practice, this package makes you be less productive in speed :)

The goal of the package is the learning and practicing [The Clean Architecture](https://8thlight.com/blog/uncle-bob/2012/08/13/the-clean-architecture.html).
By doing so you will create web/cli/{{ .ChannelName }} applications that actually external channel and framework independent.

By working disciplined and seperate the scope of your code and enforcing [law of demeter](https://en.wikipedia.org/wiki/Law_of_Demeter) on your architecture,
by my you probably end up with something that is boring in term of code, not have fancy.

The results will be something like:

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

*Yes but how will this help me to achieve this ?*

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
