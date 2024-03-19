package querykit

type Visitor[GatewayQuery any] func(node Node) (GatewayQuery, error)

func Visit[GatewayQuery any, Entity any](b Builder[GatewayQuery, Entity], visitor Visitor[GatewayQuery]) (GatewayQuery, error) {
	return visitor(b.node)
}
