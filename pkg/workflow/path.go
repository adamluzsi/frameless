package workflow

import (
	"context"
	"iter"
	"slices"

	"go.llib.dev/frameless/pkg/contextkit"
)

type ctxKeyPathRoot struct{}
type ctxKeyPath struct{}

var pathNodeH contextkit.ValueHandler[ctxKeyPath, pathNode]

type pathNode struct {
	Parent *pathNode
	Name   string
}

func (n pathNode) FromRoot() iter.Seq[*pathNode] {
	return func(yield func(*pathNode) bool) {
		var nodes []*pathNode
		var current *pathNode = &n
		for {
			nodes = append(nodes, current)
			if current.Parent != nil {
				current = current.Parent
				continue
			}
			break
		}
		slices.Reverse(nodes)
		for _, node := range nodes {
			if !yield(node) {
				return
			}
		}
	}
}

type Path []string

func (p Path) Equal(oth Path) bool {
	return slices.Equal(p, oth)
}

func CurrentPath(ctx context.Context) Path {
	node, ok := pathNodeH.Lookup(ctx)
	if !ok {
		return nil
	}
	var path Path
	for node := range node.FromRoot() {
		path = append(path, node.Name)
	}
	return path
}

func WithName(ctx context.Context, name string) context.Context {
	next := pathNode{Name: name}
	if node, ok := pathNodeH.Lookup(ctx); ok {
		next.Parent = &node
	}
	return pathNodeH.ContextWith(ctx, next)
}
