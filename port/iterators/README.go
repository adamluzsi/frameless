/*
Package iterators provide iterator implementations.

# Summary

An Iterator's goal is to decouple the origin of the data from the consumer who uses that data.
Most commonly, iterators hide whether the data comes from a specific database, standard input, or elsewhere.
This approach helps to design data consumers that are not dependent on the concrete implementation of the data source,
while still allowing for the composition and various actions on the received data stream.
An Iterator represents an iterable list of element,
which length is not known until it is fully iterated, thus can range from zero to infinity.
As a rule of thumb, if the consumer is not the final destination of the data stream,
it should use the pipeline pattern to avoid bottlenecks with local resources such as memory.

# Resources

https://en.wikipedia.org/wiki/Iterator_pattern
https://en.wikipedia.org/wiki/Pipeline_(software)
*/
package iterators
