/*

Package frameless -> The Hitchhiker's Guide to the Grumpytecture





Pre Words

Everything you read here handle with a grain of salt, because it is just an opinion on this subject.
Therefore this specification is just an example on how to include some ideas that I learned in the hard way about design.
You have to decide in your project whether it's worth it or not adding this extra but thin abstraction layer.
So not the exact specification here is what is the most important content of this document but the ideas it tries to represent.

I tried my best and researched this idea by working on a project of mine that had it's own caveats.
During design of such caveat I have already known that I would have limited time to work on.
I intentionally tried to force myself to design a solution that breaks down small isolated parts where the required mind model can be small,
and all the task in a user story can be executed almost independently from each other by using generally appliable contracts between parts.
My most important requirement was to reduce the time I loose while I load the required mind model for a given task.
This includes the way I can test my application without falling into a trap of full green test suite for a not working application.
This approach has it's own pro and con.

To summary up this section, if you decide to layer your application but to do it on your own/team's way,
I hope my research can at least be useful on your journey.





Introduction

This research project primary was created for myself, to extend my knowledge in software design.
You will probably see a few common design principles that I prefer, and known commonly.
The core concept is based on inspiration by Robert Martin works, and on my own experience.





The reason

While I had the pleasure to work with software engineers that I respected for both personality and knowledge,
there was one commonly returning feeling that kept bothering me.
It endend in an extreme that I realized, I love minimalism in programming,
and unconsciously I started to avoid frameworks that provides rapid development with "ease of use".

Most of these framework requires disciplined following it's conventions,
which I really like for team scaleability and productivity reasons.
But in the same time, I see that software design suffer in the long run.
Project gets tightly chained to that framework,
Business entities, use cases and rules depending on the framework, and its smart objects.

This in the end resulted in a way where the Juniors who "grew up"* working with these tools,
tends to have some tight connection with a framework rather than core ideas.
I'm in no position to define this is good or not. The software made this way could be really high quality.
However, they usually suffer in testability in a way, that it requires tons of external resources to test business use cases, which tends to give slow test suite.
Features created in a way where use cases implemented after choosing backing resources feels like building a house by starting from the roof.

But because people expecting results to be delivered, and if possible by yesterday, It's rare to have a moment to think every project through.
And because I like to think through things, play in mind what could be the future outcome of a given decision...
How will it affect maintainability ?
How easy will the code to be consumed if someone join the team of that project and want to read it alone ?
How much effort will be required to build mind model for the code ?





The Goal

So I decided to find time in my life and research out in my own speed a solution for the above mentioned problem.
I also tried to include the factor that we software engineers like play with new toys (abstractions)
and I would like to leave space for that for myself as well. So I tried layer software into the following:

• Entity
Is usually a data structure that may or may not have functions.

• Controller
that implements a specific business use case.
Depends on Entities it works, it defines what the presenter receives,
also defines what query use cases the storage must implement.

• Query Use Case
for controllers that have to interact with a storage.
This defines what is the expected behavior to be implemented in the storage or storages (for example dark launch).
It depends on the Entity, which it works with.

• Resuest and Presenter
implements the interaction between an input external interface and an output external interface that could even be the same as well.
Request implements the unserialization logic required to have only primitives as input.
Presenter implements the serialization logic that will be used in the outbound communication from the application.
This two interface remove the above mentioned responsibility from the controller, so as a result, controller wil only have use case controlling related logic.
I believe this makes testing and composition of these parts easier, while the required mind model will be super little for controllers as well.

• Storage
implements query use case specifications on a specific external resource that main purpose is storing data.
Hides the technology/packages from the controller, so the dependency inversion can be enforced even more.
Ensure that the given external resource connection only used through this and it is not leaked out.
Your entities by default will not know about their persistence, the only information can optionally exchange is the storage ID.
As a nice side effect, because storage is a dependency for controller rather than encapsulation in an entity, like how most ORM frameworks do,
you can use during testing in-memory storages, so your red/green/refactor feedback cycle will be blazing fast,
and you can start create business use cases implementations without any storage.





Principles that should be followed when you design with this package

Rule 1.
You can't tell where a program is going to spend its time.
Bottlenecks occur in surprising places, so don't try to guess and build in a speed hack until you've proven that's where the bottleneck really is.

Rule 2.
Measure. Don't tune for speed until you've measured, and even then, still don't tune unless one part of the code overwhelms the rest.

Rule 3.
Fancy algorithms are slow when n is small,
and n is usually small. Fancy algorithms have big constants.
Until you know that n is frequently going to be big, don't get fancy. (Even if n does get big, use Rule 2 first.)

Rule 4.
Fancy algorithms are buggier than simple ones, and they're much harder to implement. Use simple algorithms as well as simple data structures.

Rule 5.
Data dominates. If you've chosen the right data structures and organized things well, the algorithms will almost always be self-evident.
Data structures, not algorithms, are central to programming.

Rule 6.
Don't create production code that is not proven to be required.
Prove it with user story and tests.
If it is "hard to test", take a break and think over.

Rule 7.
Use contracts wherever it makes sense instead of concrete type declarations to enforce dependency inversion.

Rule 8.
Try creating code parts that a stranger could understand within 15m,
else try to reduce the code in a way that it require lesser mind model.

Most of the rules originated from one of my favorite programmer,
who was the initial reason why I started to read about golang, Rob Pike.
Pike's rules 1 and 2 restate Tony Hoare's famous maxim "Premature optimization is the root of all evil."
Ken Thompson rephrased Pike's rules 3 and 4 as "When in doubt, use brute force.".
Rules 3 and 4 are instances of the design philosophy KISS.
Rule 5 was previously stated by Fred Brooks in The Mythical Man-Month.





Caveat

This research primary targets creating design for business software applications.
If you need to create a service for technology specific an edge case purpose,
than you probably better of without the extra layer this requires.

	such case is a HTTP reverse proxy with custom logic related to the transferred data.

Also the design here strongly assumes that you create software that is tested and follows 12 factor principles, so it's scalable via the process model.
I worked with languages that are way slower than golang, so I have a different view about "required" performance,
and I don't share the opinion that the application must be prematurely optimized for some extra nano seconds.
Therefore for those who benchmark interface{} vs struct{} method execution speed may find this package disturbing.





Why Golang

Most of these experience sourced from working in other languages,
and programming in golang's minimalist environment is kind a like a chill out place for my mind,
but the knowledge I try to form it into code here is language independent and could be used in any other languages as well.
In languages where there are no interfaces, I highly recommend creating specification that ensure this simple contract.





Last notes

As a last note, most of the interfaces defined here may only contain a few or just one function signature,
it is because I tried remove everything that is YAGNI in order to achieve final goal for a given project.
QueryUseCase is the tipical example for this, because you only implement those that you use it. and nothing more.
I would like to ask you, if you see anything and care to share your constructive opinion,
please feel free to create an issue on github where we can discuss this!





Resources

https://12factor.net/
https://en.wikipedia.org/wiki/Law_of_Demeter
https://golang.org/pkg/encoding/json/#Decoder
https://en.wikipedia.org/wiki/Iterator_pattern
https://en.wikipedia.org/wiki/Adapter_pattern
https://en.wikipedia.org/wiki/You_aren%27t_gonna_need_it
https://en.wikipedia.org/wiki/Single_responsibility_principle
https://8thlight.com/blog/uncle-bob/2012/08/13/the-clean-architecture.html


*/
package frameless
