/*

Package frameless aims to create convention to build software from ground instead of upside down.

What does this in in practice

You can practice building software with clean seperation between layers.
For example you can implement Business Usecases without any external dependency.
I often saw that a developer like to challenge they mind and also to play around with new hyped modules or frameworks.
In a software where there is no strict seperation between external interfaces and internal business rules / use cases,
this easily mean that the software development will be throttled over time because reverse dependency injection.

Initially the goal for this project was starting to extend my vision in software level architecture
and find a way out for myself what is "clean" to me in terms of Architecture.
As a side effect my software become even more easier to test, than before.
By creating like this my software, it will be framework independent, and I can use whatever I want for my software channel (web/cli/mq/etc).

I'm not saying this is the silver bullet here, because the actually usable real world usecases limited to business applications.
If you need to create a service for an edge case purpose, than you probably better of without extra complexity layer,
because would not make it cleaner, just more complex. Such case is building a reverse proxy that handles custom security rules and stuff like that.
Also the design here highly assumes you create software that follows 12 factor principles, and scalable via the process model.
I worked with languages that are anything but high-performant, so I have a different view about "required" performance,
and I don't share the opinion that the application must be prematurely optimized for some extra nano seconds.
Even with one of the slowest languages in the world you can architect and build scalable and quick business softwares,
so golang is chosen for different reasons to be one of my favorite language.

My main goals while design a business applications is maintainability, testability, scope limitation and testability for components.
To me if golang would be slow I still would love to use it, because I really like how opinionated the language is.

If you feel while using idioms from here, that your test are too simple and boring, than I already happy.
Of course we not live in a world where every company open to give extra time to achieve this,
so I started this project as a guideline to make myself and hopefully others able to create applications in a fast and efficient way.
I try to create primary for myself conventions that on the long run help avoid common mistakes that usually crystalize out not when being developed,
but when a software have to be updated later.
Even if you are the one who have to update it, after a certain amount of time it can be easily happen that you easily end up watching your own code like a stranger would do.
Usually the smaller the required mind model to be built, the faster you can interact with an application code base.
And this is what this "meta" framework try to achieve.

Therefore if your opinion includes any of the followings:
 * I don't mind using interfaces
 * I don't care speed differences between function calls on interface <=> struct
 * I don't depend purely only on type check, I'm comfortable with tests as well.
If you said yes to this, I guess there would be no harm continue reading this.


Last notes

As a last note, most of the interfaces defined here may look simple,
but took much more time than it looks, so sometimes one function signature was created under days.
I would like to ask you, if you see anything and care to share your constructive opinion,
please feel free to create an issue where it can be discussed!


Resources

https://12factor.net/
https://en.wikipedia.org/wiki/Law_of_Demeter
https://golang.org/pkg/encoding/json/#Decoder
https://en.wikipedia.org/wiki/Iterator_pattern
https://en.wikipedia.org/wiki/Adapter_pattern
https://en.wikipedia.org/wiki/You_aren%27t_gonna_need_it
https://en.wikipedia.org/wiki/Single_responsibility_principle
https://8thlight.com/blog/uncle-bob/2012/08/13/the-clean-architecture.html

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

Example Entity:

		type User struct {
			ID        string
			Name      string
			Email     string
			Biography string
		}


*/
package frameless
