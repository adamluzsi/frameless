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
