package httpkit

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"maps"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strings"

	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/pathkit"
	rf "go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
)

func NewRouter(configure ...func(*Router)) *Router {
	router := &Router{}
	for _, c := range configure {
		c(router)
	}
	return router
}

type Router struct {
	root *_Node
}

type _Node struct {
	middlewares []MiddlewareFactoryFunc
	methodsH    map[string] /* method */ http.Handler
	defaultH    http.Handler

	fixNodes _FixNodes
	dynNodes _DynNodes

	mux *http.ServeMux
}

// _FixNodes are the literal path segments, keyed by their unescaped value.
type _FixNodes map[string] /* path */ *_Node

// _DynNodes are the path parameter placeholders of a path level, in the order they were registered.
type _DynNodes []_DynNode

type _DynNode struct {
	VarName string
	Node    *_Node
}

func (dns *_DynNodes) NodeFor(varName string) *_Node {
	for _, dn := range *dns {
		if dn.VarName == varName {
			return dn.Node
		}
	}
	node := &_Node{}
	node.init()
	*dns = append(*dns, _DynNode{VarName: varName, Node: node})
	return node
}

// children yields the sub nodes that match a raw (escaped) request path segment,
// together with the context that holds the path parameter of a placeholder node.
//
// A literal segment is matched before the placeholders.
// When more placeholders could match, the one registered last is tried first,
// the same way registering a route again replaces the earlier one.
func (r *_Node) children(ctx context.Context, rawpathpart string) iter.Seq2[context.Context, *_Node] {
	return func(yield func(context.Context, *_Node) bool) {
		value := unescapePathSegment(rawpathpart)
		if child, ok := r.fixNodes[value]; ok {
			if !yield(ctx, child) {
				return
			}
		}
		for i := len(r.dynNodes) - 1; 0 <= i; i-- {
			dn := r.dynNodes[i]
			if !yield(WithPathParam(ctx, dn.VarName, value), dn.Node) {
				return
			}
		}
	}
}

func unescapePathSegment(raw string) string {
	if v, err := url.PathUnescape(raw); err == nil {
		return v
	}
	return raw
}

func (r *_Node) Merge(oth *_Node) {
	if oth == nil {
		return
	}
	r.init()
	for k, v := range oth.methodsH {
		r.methodsH[k] = v
	}
	for k, v := range oth.fixNodes {
		r.fixNodes[k] = v
	}
	if oth.mux != nil {
		if r.mux == nil {
			r.mux = oth.mux
		} else {
			r.mux.Handle("/", oth.mux)
		}
	}
	if 0 < len(oth.middlewares) {
		r.middlewares = append(r.middlewares, oth.middlewares...)
	}
	for _, dn := range oth.dynNodes {
		r.dynNodes.NodeFor(dn.VarName).Merge(dn.Node)
	}
}

func (r *_Node) init() {
	if r.methodsH == nil {
		r.methodsH = make(map[string]http.Handler)
	}
	if r.fixNodes == nil {
		r.fixNodes = make(_FixNodes)
	}
}

// methodHandler returns the handler of the route registered for the method.
// Like with http.ServeMux, a GET route serves the HEAD requests too, unless a HEAD route is registered,
// and net/http's server discards the body of the reply.
func (r *_Node) methodHandler(method string) (http.Handler, bool) {
	if handler, ok := r.methodsH[method]; ok {
		return handler, true
	}
	if method == http.MethodHead {
		handler, ok := r.methodsH[http.MethodGet]
		return handler, ok
	}
	return nil, false
}

// allowedMethods lists the methods that the node has a route for, in the order of httpMethodOrdering.
func (r *_Node) allowedMethods() []string {
	methods := slices.Collect(maps.Keys(r.methodsH))
	if _, ok := r.methodHandler(http.MethodHead); ok && !slices.Contains(methods, http.MethodHead) {
		methods = append(methods, http.MethodHead)
	}
	slices.SortFunc(methods, compareHTTPMethods)
	return methods
}

// ServeHTTP routes the request to the handler that matches the request path.
//
// A route registered with On (Get, Post, etc.) only matches its own path,
// and when it is registered for other methods only, the reply is 405 Method Not Allowed.
// A handler registered with Mount, Resource or Handle matches its path and every path under it.
// It is used when no route matches the request,
// and the one closest to the requested path wins.
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if router.root == nil {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
		return
	}
	r, route := internal.WithRoutingContext(r)
	m := _Matcher{
		Method:   r.Method,
		Segments: pathkit.Split(route.PathLeft),
	}
	match, ok := m.Route(router.root, r.Context())
	if !ok {
		handler, mws := m.NoMatch()
		WithMiddleware(handler, mws...).ServeHTTP(w, r)
		return
	}
	route.Travel(pathkit.Join(m.Segments[:match.Depth]...))
	r = r.WithContext(match.Context)
	handler := match.Handler
	if match.Prefix && route.Current != "/" {
		// a mounted handler receives the request path relative to its mount point.
		handler = withRequestPath(route.PathLeft, handler)
	}
	WithMiddleware(handler, match.Middlewares...).ServeHTTP(w, r)
}

type _Matcher struct {
	Method string
	// Segments are the raw (escaped) segments of the request path.
	Segments []string

	// allowed holds the methods of the first route that matched the path, but not the method.
	allowed    []string
	allowedMWs []MiddlewareFactoryFunc

	// deepest is the node that matched the most of the request path.
	deepestDepth int
	deepestMWs   []MiddlewareFactoryFunc
}

type _Match struct {
	Handler     http.Handler
	Context     context.Context
	Middlewares []MiddlewareFactoryFunc
	// Depth is the number of path segments the match consumed.
	Depth int
	// Prefix tells that Handler is a mounted handler, which handles the paths under it as well.
	Prefix bool
}

func (m *_Matcher) Route(root *_Node, ctx context.Context) (_Match, bool) {
	m.deepestDepth = -1
	if match, ok := m.exact(root, ctx, 0, nil); ok {
		return match, true
	}
	return m.prefix(root, ctx, 0, nil)
}

// exact looks up a handler for the entire request path.
func (m *_Matcher) exact(node *_Node, ctx context.Context, depth int, mws []MiddlewareFactoryFunc) (_Match, bool) {
	mws = append(slices.Clip(mws), node.middlewares...)
	if m.deepestDepth < depth {
		m.deepestDepth = depth
		m.deepestMWs = mws
	}
	if depth == len(m.Segments) {
		if handler, ok := node.methodHandler(m.Method); ok {
			return _Match{Handler: handler, Context: ctx, Middlewares: mws, Depth: depth}, true
		}
		if node.defaultH != nil {
			return _Match{Handler: node.defaultH, Context: ctx, Middlewares: mws, Depth: depth, Prefix: true}, true
		}
		if 0 < len(node.methodsH) && m.allowed == nil {
			m.allowed = node.allowedMethods()
			m.allowedMWs = mws
		}
		return _Match{}, false
	}
	for sctx, snode := range node.children(ctx, m.Segments[depth]) {
		if match, ok := m.exact(snode, sctx, depth+1, mws); ok {
			return match, true
		}
	}
	return _Match{}, false
}

// prefix looks up the mounted handler closest to the request path.
func (m *_Matcher) prefix(node *_Node, ctx context.Context, depth int, mws []MiddlewareFactoryFunc) (_Match, bool) {
	mws = append(slices.Clip(mws), node.middlewares...)
	if depth < len(m.Segments) {
		for sctx, snode := range node.children(ctx, m.Segments[depth]) {
			if match, ok := m.prefix(snode, sctx, depth+1, mws); ok {
				return match, true
			}
		}
	}
	if node.defaultH != nil {
		return _Match{Handler: node.defaultH, Context: ctx, Middlewares: mws, Depth: depth, Prefix: true}, true
	}
	if node.mux != nil {
		return _Match{Handler: node.mux, Context: ctx, Middlewares: mws, Depth: depth, Prefix: true}, true
	}
	return _Match{}, false
}

// NoMatch returns the handler that replies when no handler matched the request.
func (m *_Matcher) NoMatch() (http.Handler, []MiddlewareFactoryFunc) {
	if 0 < len(m.allowed) {
		allow := strings.Join(m.allowed, ", ")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", allow)
			defaultErrorHandler.HandleError(w, r, ErrMethodNotAllowed)
		}), m.allowedMWs
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
	}), m.deepestMWs
}

// withRequestPath makes the next handler receive the request with the given escaped path.
func withRequestPath(escapedPath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		if p, err := url.ParseRequestURI(escapedPath); err == nil {
			u.Path, u.RawPath = p.Path, p.RawPath
		} else {
			u.Path, u.RawPath = escapedPath, ""
		}
		nr := new(http.Request)
		*nr = *r
		nr.URL = &u
		next.ServeHTTP(w, nr)
	})
}

// Mount will mount a handler to the router.
// The mounted handler receives the requests of the path and of every path under it,
// unless a route matches the request more closely.
// The handler receives the request path relative to the mount point,
// so for Mount("/foo", h), the request to /foo/bar/baz reaches h as /bar/baz.
//
// When the handler is a *Router, its routes are merged into the router under the path.
func (router *Router) Mount(path string, handler http.Handler) {
	ro := router.mkpath(path)
	switch h := handler.(type) {
	case *Router:
		ro.Merge(h.root)
	default:
		ro.defaultH = h
	}
}

func (router *Router) Namespace(path string, blk func(ro *Router)) {
	blk(router.Sub(path))
}

func (router *Router) Sub(path string) *Router {
	return &Router{root: router.mkpath(path)}
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, Handle panics.
func (router *Router) Handle(pattern string, handler http.Handler) {
	router.init()
	switch h := handler.(type) {
	case *Router:
		ro := router.mkpath(pattern)
		ro.Merge(h.root)
	default:
		// alternatively, you can traverse the path,
		// and memorise what path must be stripped from the handler
		ro := router.root
		if ro.mux == nil {
			ro.mux = http.NewServeMux()
		}
		ro.mux.Handle(pattern, handler)
	}
}

func (router *Router) init() {
	if router.root == nil {
		router.root = &_Node{}
	}
	router.root.init()
}

func (router *Router) mkpath(path string) *_Node {
	router.init()
	var ro *_Node
	ro = router.root
	for _, part := range pathkit.Split(path) {
		ro.init()
		if vn, ok := isPathParamPlaceholder(part); ok {
			ro = ro.dynNodes.NodeFor(vn)
			continue
		}
		// like with http.ServeMux, a path segment is matched by its unescaped value.
		part = unescapePathSegment(part)
		if _, ok := ro.fixNodes[part]; !ok {
			ro.fixNodes[part] = &_Node{}
		}
		ro = ro.fixNodes[part]
	}
	return ro
}

// On registers a handler for a method on the path.
// The handler only receives requests of the exact path,
// with the path parameters of the path's placeholders, like "/users/:id".
func (router *Router) On(method, path string, handler http.Handler) {
	router.Namespace(path, func(r *Router) {
		r.init()
		r.root.methodsH[method] = handler
	})
}

func (router *Router) Get(path string, handler http.Handler) {
	router.On(http.MethodGet, path, handler)
}

func (router *Router) Post(path string, handler http.Handler) {
	router.On(http.MethodPost, path, handler)
}

func (router *Router) Delete(path string, handler http.Handler) {
	router.On(http.MethodDelete, path, handler)
}

func (router *Router) Put(path string, handler http.Handler) {
	router.On(http.MethodPut, path, handler)
}

func (router *Router) Patch(path string, handler http.Handler) {
	router.On(http.MethodPatch, path, handler)
}

func (router *Router) Head(path string, handler http.Handler) {
	router.On(http.MethodHead, path, handler)
}

func (router *Router) Connect(path string, handler http.Handler) {
	router.On(http.MethodConnect, path, handler)
}

func (router *Router) Options(path string, handler http.Handler) {
	router.On(http.MethodOptions, path, handler)
}

func (router *Router) Trace(path string, handler http.Handler) {
	router.On(http.MethodTrace, path, handler)
}

// Resource will register a restful resource path using the Resource handler.
//
// Paths for Router.Resource("/users", httpkit.RESTHandler[User, UserID]):
//
//	INDEX	- GET 		/users
//	CREATE	- POST 		/users
//	SHOW 	- GET 		/users/:id
//	UPDATE	- PUT 		/users/:id
//	DESTROY	- DELETE	/users/:id
func (router *Router) Resource(identifier string, h restHandler) {
	router.Mount(pathkit.Canonical(identifier), h)
}

// restHandler is a generic interface for the typesafe RESTHandler[ENT, ID]
type restHandler interface {
	restHandler()
	http.Handler
}

// Use will instruct the router to use a given MiddlewareFactoryFunc to
func (router *Router) Use(mws ...MiddlewareFactoryFunc) {
	router.init()
	router.root.middlewares = append(router.root.middlewares, mws...)
}

func (router *Router) RouteInfo() RouteInfo {
	return nodeRoutes(router.root)
}

///////////////////////////////////////////////////////////////////////////////////////////////////

const allPathInfoMethod = "ALL"

func GetRouteInfo(h http.Handler) RouteInfo {
	if h == nil {
		return nil
	}
	if docs, ok := h.(RouteInformer); ok {
		return docs.RouteInfo()
	}
	var httpHandlerType = reflect.TypeOf(h)
	if doc, ok := riReg.Lookup(httpHandlerType); ok {
		return doc(h)
	}
	// by default a handler will receive all call
	return []PathInfo{{Method: allPathInfoMethod, Path: "/"}}
}

var riReg synckit.Map[reflect.Type, func(h http.Handler) RouteInfo]

func RegisterRouteInformer[T http.Handler](fn func(v T) RouteInfo) func() {
	var httpHandlerType = rf.TypeOf[T]()
	riReg.Set(httpHandlerType, func(h http.Handler) RouteInfo {
		return fn(h.(T))
	})
	return func() { riReg.Delete(httpHandlerType) }
}

type RouteInfo []PathInfo

type PathInfo struct {
	Method string `enum:"ALL,POST,GET,PUT,PATCH,DELETE,"`
	Path   string
	Desc   string
}

func (ri RouteInfo) String() string {
	var width int
	for _, s := range ri {
		if l := len(s.Method); l > width {
			width = l
		}
	}
	var rs []string
	for _, r := range ri {
		rs = append(rs, fmt.Sprintf("%-*s %s", width, r.Method, r.Path))
	}
	return strings.Join(rs, "\n")
}

func (ri RouteInfo) WithMountPoint(MountPoint string) RouteInfo {
	return slicekit.Map(ri, func(pi PathInfo) PathInfo {
		var lastChar rune
		for _, char := range pi.Path {
			lastChar = char
		}
		ogPathLen := len(pi.Path)
		pi.Path = pathkit.Join(MountPoint, pi.Path)
		if lastChar == '/' && 1 < ogPathLen {
			pi.Path += "/"
		}
		return pi
	})
}

type RouteInformer interface {
	RouteInfo() RouteInfo
}

func nodeRoutes(node *_Node) RouteInfo {
	var ri RouteInfo
	if node == nil {
		return ri
	}
	{ // root endpoints
		var entries []PathInfo
		for method := range node.methodsH {
			entries = append(entries, PathInfo{Method: method, Path: "/"})
		}
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			return httpMethodPriority(a.Method) < httpMethodPriority(b.Method)
		})
		ri = append(ri, entries...)
	}
	{ // fixed endpoints
		subRoutes := mapkit.ToSlice(node.fixNodes)
		sort.Slice(subRoutes, func(i, j int) bool {
			a, b := subRoutes[i], subRoutes[j]
			return a.Key < b.Key
		})
		var entries []PathInfo
		for _, re := range subRoutes {
			srs := nodeRoutes(re.Value).WithMountPoint(re.Key)
			entries = append(entries, srs...)
		}
		ri = append(ri, entries...)
	}
	{ // dynamic paths
		for _, dn := range node.dynNodes {
			ri = append(ri, nodeRoutes(dn.Node).WithMountPoint(":"+dn.VarName)...)
		}
	}
	{ // default handler
		if node.defaultH != nil {
			ri = append(ri, GetRouteInfo(node.defaultH)...)
		}
	}
	{ // http.ServeMux
		if node.mux != nil {
			ri = append(ri, GetRouteInfo(node.mux)...)
		}
	}
	return ri
}

var _ = RegisterRouteInformer[*http.ServeMux](httpServeMuxRouteInfo)

func httpServeMuxRoutingNodeRouteInfo(v reflect.Value) RouteInfo {
	if rf.IsNil(v) {
		return nil
	}

	v = rf.BaseValue(v) // accept both routingNode and *routingNode
	var path string

	var patternToString = func(pattern reflect.Value) string {
		if rf.IsNil(pattern) {
			return "/"
		}
		pattern = rf.BaseValue(pattern)
		_, part, _ := rf.LookupField(pattern, "str")
		return part.String()
	}

	if _, patternPtr, ok := rf.LookupField(v, "pattern"); ok && !rf.IsNil(patternPtr) {
		path = patternToString(patternPtr)
	}

	var ri RouteInfo

	if _, handler, ok := rf.LookupField(v, "handler"); ok && !rf.IsNil(handler) {
		if handler.CanInterface() {
			if h, ok := handler.Interface().(http.Handler); ok {
				nri := GetRouteInfo(h).WithMountPoint(path)
				ri = append(ri, nri...)
			} else {
				ri = append(ri, PathInfo{
					Method: allPathInfoMethod,
					Path:   path,
				})
			}
		} else {
			ri = append(ri, PathInfo{
				Method: allPathInfoMethod,
				Path:   path,
			})
		}
	}

	if _, children, ok := rf.LookupField(v, "children"); ok && !rf.IsNil(children) {
		if _, s, ok := rf.LookupField(children, "s"); ok && !rf.IsNil(s) {
			for _, entry := range rf.IterSlice(s) {
				_, routingNodePtr, ok := rf.LookupField(entry, "value") /* *routingNode */
				if !ok {
					continue
				}

				ri = append(ri, httpServeMuxRoutingNodeRouteInfo(routingNodePtr)...)
			}
		}
		if _, m, ok := rf.LookupField(children, "m"); ok && !rf.IsEmpty(m) { // map[string, *routingNode]
			for _, routingNodePtr := range rf.IterMap(m) {
				ri = append(ri, httpServeMuxRoutingNodeRouteInfo(routingNodePtr)...)
			}
		}
	}

	if _, multiChild, ok := rf.LookupField(v, "multiChild"); ok && !rf.IsNil(multiChild) {
		ri = append(ri, httpServeMuxRoutingNodeRouteInfo(multiChild)...)
	}

	if _, emptyChild, ok := rf.LookupField(v, "emptyChild"); ok && !rf.IsNil(emptyChild) {
		ri = append(ri, httpServeMuxRoutingNodeRouteInfo(emptyChild)...)
	}

	return ri

}
func httpServeMuxRouteInfo(mux *http.ServeMux) RouteInfo {
	var rmux reflect.Value
	rmux = reflect.ValueOf(mux)
	rmux = rf.BaseValue(rmux)
	var ri RouteInfo
	if _, tree, ok := rf.LookupField(rmux, "tree"); ok {
		ri = append(ri, httpServeMuxRoutingNodeRouteInfo(tree)...)
	}
	return slicekit.Unique(ri)
}

func httpMethodPriority(method string) int {
	for i, comp := range httpMethodOrdering {
		if method == comp {
			return i + 1
		}
	}
	return math.MaxInt
}

func compareHTTPMethods(a, b string) int {
	if c := cmp.Compare(httpMethodPriority(a), httpMethodPriority(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

var httpMethodOrdering = []string{
	http.MethodPost,   // Create
	http.MethodGet,    // Read
	http.MethodPut,    // Update
	http.MethodPatch,  //
	http.MethodDelete, // destroy

	http.MethodHead,
	http.MethodConnect,
	http.MethodOptions,
	http.MethodTrace,
}
