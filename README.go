/*

Package frameless - The Hitchhiker's Guide to the Grumpytecture.


Introduction

Everything you read here handle with a grain of salt because it is just my personal opinion on this subject.
This research project primary was created for myself, to refresh and sort out my experience about design for my self.


The reason

There is a nice trade-off between using certain conventions or a framework that try to collect experience
and build safety guards for those who may not experienced the same pitfalls yet.
Using a framework on hand allows new recruits to able to create value for the company,
on the other hand, hides a lot of internals from them
and prevents understanding certain problems from simplicity point of view.
As a side effect, frameworks often leak into the design of the application.
It is nice to have something that able to provide ability to avoid common pitfalls,
but I believe, that it is important to keep in front of our eye,
that if we have a hammer, we should not think about every problem as nails.
There is nothing wrong using frameworks, but we have to make sure,
they are only used for a certain problem only, and not for gluing together everything.
For example, if a framework provide help handling HTTP requests,
we should not over using it, to external resources (like DB), business logic (use-cases) and business entities.
As long we discipline ourselves to keep everything for what it meant to,
we will receive maintainability, observability and minimal mental model needs in our project.
Trough this, we can make sure, that our application use certain knowledge from the framework,
and not an "xy framework app" that is heavily vendor locked to that framework.
Having to deep connection with a framework can easily cause our application
to be volatile against breaking changes in the framework.

This conventions that I collected in this project are serve me one purpose,
which is to overcome the fact that as a human, we are in generally bad at programming.
We have limited mental model capacity, and it takes time to build it up, and if one mistake made during that,
the wrongly build mental model will cause bugs, and it needs minimum rubber duck debugging to fix the model.
And to fight this and similar problems all the techniques here aim to minimise during programming
the required mental model, help practice roles alone such as the "driver" and the "navigator"
and in general to design the application in a way, where existing code base more or less protected from changes.
The later is especially useful, because modern programming relies on scientific approach to test the system,
and providing full edge case coverage can be exponentially hard.
Don't mix it together with the test coverage % that only check whether the code path
is being used once from a test case or not.
Therefore when a code being used by the masses (users), it is likely used in a way
that we didn't explicitly specified, and in order to not break expected system behaviors,
minimising the need to change existing code base can help in general.
I heavily sympathise on TDD/BDD but even with a full % coverage,
the edge cases between components are in general harder to test than in simple unit tests with contracts.

The quality of the software in this project therefore defined by factors as
how likely you have to change existing code base,
how fast you receive back feedback regarding your change,
at what quality level it able to provide feedback about system behavior changes,
and how big mental model you need to build in order to understand the application on high level.


The Pressure

Because different stakeholders expect results to be delivered, and if possible as soon as possible,
It's rare to have a moment to think every decision through in a life of a project.
The most often challenging ones are decisions made to avoid some boilerplate in the name of minimalism,
but on the long run results in interface violations because rewrites.

I like to think through things, play in mind that what could be the future outcome of a given decision...
How will it affect maintainability?
How easy will the code be consumed if someone joins the team of that project and want to read it alone?
How much effort will be required to build the mental model for the code?
These are the questions I like to sort out beforehand by building principles that help me keep my self disciplined.

You will not find anything regarding complete out of the box solutions,
just some idea and practice I like to follow while working on projects.
Please don't expect in this repo examples in a way, that you can easily wire into your project.
You can check projects that use idioms from here if you interested in such examples.



Why Golang

Most of these experience sourced from working in other languages,
and programming in golang's minimalist environment is kinda like a chill out place for my mind,
but the knowledge I try to form it into the code here is language independent and could be used in any other languages as well.
In languages where there are no interfaces, I highly recommend creating the specification that ensure this simple contract.



Principles that I liked and try to follow when I design

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
else try to reduce the code in a way that it requires a lesser mind model.

Most of the rules originated from one of my favorite programmers,
who was the initial reason why I started to read about golang, Rob Pike.
Pike's rules 1 and 2 restate Tony Hoare's famous maxim "Premature optimization is the root of all evil."
Ken Thompson rephrased Pike's rules 3 and 4 as "When in doubt, use brute force.".
Rules 3 and 4 are instances of the design philosophy KISS.
Rule 5 was previously stated by Fred Brooks in The Mythical Man-Month.



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
