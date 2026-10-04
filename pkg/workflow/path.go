package workflow

import (
	"context"
	"iter"
	"slices"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type Path []Scope

type Scope struct {
	Name string
	Var  bool
}

func (s Scope) Equal(oth Scope) bool {
	return s.Name == oth.Name
}

func (p Path) Equal(oth Path) bool {
	return slicekit.Equal(p, oth)
}

// MatchPrefix reports whether the path begins with the given prefix.
func (p Path) MatchPrefix(prefix Path) bool {
	return slicekit.MatchPrefix(p, prefix)
}

//--- CurrentPath ---

func CurrentPath(ctx context.Context) Path {
	return getPath[Path](ctx, ctxCurrentPath)
}

func WithName(ctx context.Context, name string) context.Context {
	return withName(ctx, ctxCurrentPath, name)
}

var ctxCurrentPath contextkit.ValueHandler[ctxKeyCurrentPath, pathNode]

type ctxKeyCurrentPath struct{}

// --- VarScope --- //

type VarScope []string

func CurrentVarScope(ctx context.Context) VarScope {
	return getPath[VarScope](ctx, ctxVarScope)
}

func WithVarScope(ctx context.Context, name string) context.Context {
	return withName(ctx, ctxVarScope, name)
}

var ctxVarScope contextkit.ValueHandler[ctxKeyVarScope, pathNode]

type ctxKeyVarScope struct{}

// --- utils --- //

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

func getPath[S ~[]string, K ~struct{}](ctx context.Context, h contextkit.ValueHandler[K, pathNode]) S {
	node, ok := h.Lookup(ctx)
	if !ok {
		return nil
	}
	var p S
	for node := range node.FromRoot() {
		p = append(p, node.Name)
	}
	return p
}

func withName[K ~struct{}](ctx context.Context, h contextkit.ValueHandler[K, pathNode], name string) context.Context {
	var next = pathNode{Name: name}
	if node, ok := h.Lookup(ctx); ok {
		next.Parent = &node
	}
	return h.ContextWith(ctx, next)
}

// withPath replaces the chain held under h with the given names, from the root.
// An empty path clears the chain.
func withPath[S ~[]string, K ~struct{}](ctx context.Context, h contextkit.ValueHandler[K, pathNode], names S) context.Context {
	var node *pathNode
	for _, name := range names {
		node = &pathNode{Parent: node, Name: name}
	}
	if node == nil {
		return h.ContextWithout(ctx)
	}
	return h.ContextWith(ctx, *node)
}
