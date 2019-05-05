/*

Package frameless - The Hitchhiker's Guide to the Grumpytecture.


Introduction

Everything you read here handle with a grain of salt because it is just my personal opinion on this subject.
This research project primary was created for myself, to refresh and sort out my experience about design for my self.


The reason

While I had the pleasure to work with software engineers that I respected for both personality and knowledge,
there was one commonly returning feeling that kept bothering me.
And It was that most frameworks that offer convention over configuration usually picture everything within the context,
therefore *vendor if developers follow those conventions blindly, they fall prey on reveres dependency inversion.
One such case when you hear a sentence like "It is risky to upgrade the xy framework version without enough time to see through everything."

Most of these framework requires disciplined following its conventions,
which I acknowledge for team saleability and productivity reasons.
But at the same time, I see that it is also counter-intuitive because if software design gets mixed up with the framework,
your team scalability may be boosted in the short term, but suffer productivity bottlenecks in the long run.
There is a countless example for this when you think about legacy applications that you would not like to touch it.
And for cases where you say it's easier to rewrite it than fix it,
there is still no guarantee that that rewriting will not suffer from the same pitfalls.

I'm in no position to define this is good or not. The software made this way could be really high quality.
All I could say that I like to have fast test builds and space to try out new technology while not affect software design by it.

But because people expecting results to be delivered, and if possible by yesterday,
It's rare to have a moment to think every project through.
And because I like to think through things, play in mind what could be the future outcome of a given decision...
How will it affect maintainability?
How easy will the code be consumed if someone joins the team of that project and want to read it alone?
How much effort will be required to build the mental model for the code?
These are the questions I like to sort out beforehand by building principles that help me keep my self disciplined.

You will not find anything regarding complete out of the box solutions,
just some idea and practice I like to follow while working on projects.



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




Last notes

As a last note, most of the interfaces defined here may only contain a few or just one function signature,
it is because I tried to remove everything that is YAGNI in order to achieve a final goal for a given project.
Query is the typical example of this because you only implement those that you use it. and nothing more.
I would like to ask you, to feel free to express your opinion, I like to listen to new perspectives,
so please feel free to create an issue on github where we can discuss this!



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
