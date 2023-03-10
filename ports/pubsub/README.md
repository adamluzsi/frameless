# Package `pubsub`

## Exchange

In a message broker system, an exchange is responsible for receiving messages from producers and routing them to the
appropriate queue or queues. The exchange does not actually store any messages, but rather determines which queues a
message should be delivered to based on the message's routing key.

When a producer sends a message to an exchange, it includes a routing key that describes the topic or category of the
message. The exchange then examines the routing key and determines which queues the message should be delivered to based
on the exchange's configuration.

There are four types of exchanges in the AMQP protocol:

- Direct exchange: A direct exchange delivers messages to queues based on the exact match of the routing key. If the
routing key of a message matches the routing key of a queue, the message is delivered to that queue.

- Fan-out exchange: A fan-out exchange delivers messages to all the queues that are bound to it. This is useful when you
want to broadcast a message to multiple consumers.

- Topic exchange: A topic exchange delivers messages to queues based on wildcard matching of the routing key.
The routing key of a message is a string with one or more words, separated by dots. Queues can bind to the exchange
using a routing key pattern that includes wildcards, such as "#" to match one or more words, or "*" to match a single
word.

Headers exchange: A headers exchange delivers messages to queues based on header values instead of the routing key.
The headers of a message are a set of key-value pairs, and queues can bind to the exchange using header matching rules.

Queues are entities that store messages in a message broker system. When a message is delivered to a queue, it is stored
in the queue until a consumer retrieves and processes the message. Queues can be bound to one or more exchanges, and
receive messages from those exchanges based on the exchange's routing rules.

In summary, exchanges are responsible for receiving messages from producers and routing them to the appropriate queues,
while queues store messages until they are processed by consumers. The exchange and queue configuration determines how
messages are routed and delivered within a message broker system.