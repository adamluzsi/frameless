/*	Package iterators provide iterator implementations.



	Summary

	An iterator goal is to decouple the facts about the origin of the data,
	to the consumer who use the data.
	Most common scenario is to hide the fact if data is from a Certain DB, STDIN or from somewhere else.
	This helps to design data consumers that doesn't rely on the data source concrete implementation,
	while still able to do composition and different kind of actions on the received data stream.
	An Iterator represent multiple data that can be 0 and infinite.
	As a general rule of thumb, if the consumer is not the final destination of the data stream,
	the consumer should use the pipeline pattern, in order to avoid bottleneck with local resources.

	Iterator define a separate object that encapsulates accessing and traversing an aggregate object.
	Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
	Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder



	Why an Object with empty interface instead of type safe channels to represent streams

	There are multiple approach to the same problem, and I only prefer this approach,
	because the error handling is easier trough this.
	In channel based pipeline pattern, you have to make sure
	that the information about the error is passed trough either trough some kind of separate error channel,
	or trough the message object it self that being passed around.
	If the pipeline can be composited during a certain use case,
	you can pass around a context.Context object to represent this.
	In the case of Iterator pattern, this failure communicated during the individual iteration,
	which leaves it up to you to propagate the error forward, or handle at the place.



	Resources

	https://en.wikipedia.org/wiki/Iterator_pattern
	https://en.wikipedia.org/wiki/Pipeline_(software)

*/
package iterators
