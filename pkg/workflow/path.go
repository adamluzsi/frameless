package workflow

import (
	"context"
	"iter"
	"slices"

	"go.llib.dev/frameless/pkg/contextkit"
)

type Path []string

func (p Path) Equal(oth Path) bool {
	return slices.Equal(p, oth)
}

func (p Path) MatchPrefix(prefix Path) bool {
	if len(p) < len(prefix) {
		return false
	}
	for i := range len(prefix) {
		if p[i] != prefix[i] {
			return false
		}
	}
	return true
}

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

// ---

var ctxCurrentPath contextkit.ValueHandler[ctxKeyCurrentPath, pathNode]

type ctxKeyCurrentPath struct{}

func getPath[K ~struct{}](ctx context.Context, h contextkit.ValueHandler[K, pathNode]) Path {
	node, ok := h.Lookup(ctx)
	if !ok {
		return nil
	}
	var path Path
	for node := range node.FromRoot() {
		path = append(path, node.Name)
	}
	return path
}

func withName[K ~struct{}](ctx context.Context, h contextkit.ValueHandler[K, pathNode], name string) context.Context {
	var next = pathNode{Name: name}
	if node, ok := h.Lookup(ctx); ok {
		next.Parent = &node
	}
	return h.ContextWith(ctx, next)
}

func CurrentPath(ctx context.Context) Path {
	return getPath(ctx, ctxCurrentPath)
}

func WithName(ctx context.Context, name string) context.Context {
	return withName(ctx, ctxCurrentPath, name)
}

//---

var ctxVarScope contextkit.ValueHandler[ctxKeyVarScope, pathNode]

type ctxKeyVarScope struct{}

func VarScope(ctx context.Context) Path {
	return getPath(ctx, ctxVarScope)
}

func WithVarScope(ctx context.Context, name string) context.Context {
	return withName(ctx, ctxVarScope, name)
}
