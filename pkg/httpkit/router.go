package httpkit

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"go.llib.dev/frameless/pkg/httpkit/internal"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/pathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
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
	dynNodes _DynNode

	mux *http.ServeMux
}

type _FixNodes map[string] /* path */ *_Node

type _DynNode struct {
	node     *_Node
	varnames []string
}

func (dn *_DynNode) NodeFor(pathParamName string) *_Node {
	if dn.node == nil {
		dn.node = &_Node{}
		dn.node.init()
	}
	if slicekit.Contains(dn.varnames, pathParamName) {
		return dn.node
	}
	dn.varnames = append(dn.varnames, pathParamName)
	return dn.node
}

func (dn *_DynNode) LookupNode(ctx context.Context, rawpathpart string) (context.Context, *_Node, bool) {
	if len(dn.varnames) == 0 || dn.node == nil {
		return nil, nil, false
	}
	for _, varname := range dn.varnames {
		varval, err := url.PathUnescape(rawpathpart)
		if err != nil {
			varval = rawpathpart
		}
		ctx = WithPathParam(ctx, varname, varval)
	}
	return ctx, dn.node, true
}

func (r *_Node) LookupNode(ctx context.Context, rawpathpart string) (context.Context, *_Node, bool) {
	if r == nil {
		return nil, nil, false
	}
	if r.fixNodes != nil {
		if sr, ok := r.fixNodes[rawpathpart]; ok {
			return ctx, sr, true
		}
	}
	if ctx, sr, ok := r.dynNodes.LookupNode(ctx, rawpathpart); ok {
		return ctx, sr, true
	}
	return nil, nil, false
}

func (r *_Node) LookupHandler(method string) (http.Handler, bool) {
	if r.methodsH != nil {
		if handler, ok := r.methodsH[method]; ok {
			return handler, ok
		}
	}
	return r.defaultH, r.defaultH != nil
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
	if oth.dynNodes.node != nil {
		for _, vn := range oth.dynNodes.varnames {
			r.dynNodes.NodeFor(vn)
		}
		r.dynNodes.node.Merge(oth.dynNodes.node)
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

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if router.root == nil {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
		return
	}
	r, route := internal.WithRoutingContext(r)
	var (
		ctx       = r.Context()
		node      = router.root
		mux       = node.mux
		muxRoute  = *route
		mws       = slicekit.Clone(router.root.middlewares)
		pathParts []string
	)
	for _, pathpart := range pathkit.Split(route.PathLeft) {
		sctx, snode, ok := node.LookupNode(ctx, pathpart)
		if !ok {
			break
		}
		if 0 < len(snode.middlewares) {
			mws = append(mws, snode.middlewares...)
		}
		pathParts = append(pathParts, pathpart)
		node = snode
		ctx = sctx
		if snode.mux != nil { // mux fallback handler
			// mux route should not include the final path part
			// since the mux contain that information if that should be handled or not
			muxRoute = route.Peek(pathkit.Join(pathParts...))
			mux = snode.mux
		}
	}
	r = r.WithContext(ctx)
	route.Travel(pathkit.Join(pathParts...))
	handler := router.toHTTPHandler(r, node, mux, muxRoute)
	handler = WithMiddleware(handler, mws...)
	handler.ServeHTTP(w, r)
}

func (router *Router) toHTTPHandler(r *http.Request, node *_Node, mux *http.ServeMux, muxRoute internal.Routing) http.Handler {
	endpoint, ok := node.LookupHandler(r.Method)
	if ok {
		return endpoint
	}
	if mux != nil {
		var handler http.Handler = mux
		if muxRoute.Current != "/" {
			handler = stripPrefix(muxRoute.Current, handler) // TODO: testme
			// handler = http.StripPrefix(muxRoute.Current, handler) // TODO: testme
		}
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultErrorHandler.HandleError(w, r, ErrPathNotFound)
	})
}

// Mount will mount a handler to the router.
// Mounting a handler will make the path observed as its root point to the handler.
// TODO: make this true :D
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
		if ro.fixNodes == nil {
			ro.fixNodes = make(_FixNodes)
		}
		if vn, ok := isPathParamPlaceholder(part); ok {
			ro = ro.dynNodes.NodeFor(vn)
			continue
		}
		if _, ok := ro.fixNodes[part]; !ok {
			ro.fixNodes[part] = &_Node{}
		}
		ro = ro.fixNodes[part]
	}
	return ro
}

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
	routes(root string) []_RouteEntry
}

// Use will instruct the router to use a given MiddlewareFactoryFunc to
func (router *Router) Use(mws ...MiddlewareFactoryFunc) {
	router.init()
	router.root.middlewares = append(router.root.middlewares, mws...)
}

func (router *Router) Routes() []string {
	routes := nodeRoutes("/", router.root)

	var width int
	for _, s := range routes {
		if l := len(s.Method); l > width {
			width = l
		}
	}

	var rs []string
	for _, r := range routes {
		rs = append(rs, fmt.Sprintf("%-*s %s", width, r.Method, r.Path))
	}

	return rs
}

type _RouteEntry struct {
	Method string `enum:"ALL,POST,GET,PUT,PATCH,DELETE,"`
	Path   string
	Desc   string
}

func nodeRoutes(root string, node *_Node) []_RouteEntry {
	var rs []_RouteEntry
	if node == nil {
		return rs
	}
	{ // root endpoints
		var entries []_RouteEntry
		for method := range node.methodsH {
			entries = append(entries, _RouteEntry{Method: method, Path: root})
		}
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			return httpMethodPriority(a.Method) < httpMethodPriority(b.Method)
		})
		rs = append(rs, entries...)
	}
	{ // fixed endpoints
		subRoutes := mapkit.ToSlice(node.fixNodes)
		sort.Slice(subRoutes, func(i, j int) bool {
			a, b := subRoutes[i], subRoutes[j]
			return a.Key < b.Key
		})
		var entries []_RouteEntry
		for _, re := range subRoutes {
			srs := nodeRoutes(pathkit.Join(root, re.Key), re.Value)
			entries = append(entries, srs...)
		}
		rs = append(rs, entries...)
	}
	{ // dynamic paths
		var (
			// :var1,:var2,:var3
			dynvarpath = strings.Join(slicekit.Map(node.dynNodes.varnames,
				func(vn string) string { return ":" + vn }), ",")
			dynroutes = nodeRoutes(pathkit.Join(root, dynvarpath), node.dynNodes.node)
		)
		rs = append(rs, dynroutes...)
	}
	{ // default handler
		if node.defaultH != nil {
			rs = append(rs, httpHandlerRoutes(root, node.defaultH)...)
		}
	}
	{ // http.ServeMux
		if node.mux != nil {
			rs = append(rs, httpMuxRoutes(root, node.mux)...)
		}
	}
	return rs
}

func httpHandlerRoutes(root string, h http.Handler) []_RouteEntry {
	if h == nil {
		return nil
	}
	switch h := h.(type) {
	case *http.ServeMux:
		return httpMuxRoutes(root, h)
	case *Router:
		return nodeRoutes(root, h.root)
	case restHandler:
		return h.routes(root)
	default:
		return []_RouteEntry{{Method: "ALL", Path: root}}
	}
}

func httpMuxRoutes(root string, mux *http.ServeMux) []_RouteEntry {
	var paths []_RouteEntry
	if mux == nil {
		return paths
	}
	// Using reflection to get the internal map

	_, m, ok := reflectkit.LookupFieldByName(reflect.ValueOf(mux).Elem(), "m")
	if !ok {
		return paths
	}

	var lookupMuxEntryHandler = func(key reflect.Value) (http.Handler, bool) {
		val := m.MapIndex(key)
		if !val.IsValid() {
			return nil, false
		}

		_, rh, ok := reflectkit.LookupFieldByName(val, "h")
		if !ok {
			return nil, false
		}

		h, ok := rh.Interface().(http.Handler)
		return h, ok
	}

iter:
	for _, key := range m.MapKeys() {
		path, ok := key.Interface().(string)
		if !ok {
			continue
		}

		var fullPath = path
		if root != "" && fullPath != "" && fullPath[0] != '/' {
			fullPath = "/" + fullPath
		}
		if root != "" {
			if fullPath != "" {
				fullPath = root + fullPath
			} else {
				fullPath = root
			}
		}

		if httpHandler, ok := lookupMuxEntryHandler(key); ok {
			// TODO: add support for handler interface subtype check
			switch h := httpHandler.(type) {
			case *Router:
				paths = append(paths, nodeRoutes(pathkit.Join(root, path), h.root)...)
				continue iter
			}
		}

		paths = append(paths, _RouteEntry{Method: "ALL", Path: fullPath})
	}

	sort.Slice(paths, func(i, j int) bool {
		a, b := paths[i], paths[j]
		return a.Path < b.Path
	})

	return paths
}

func httpMethodPriority(method string) int {
	for i, comp := range httpMethodOrdering {
		if method == comp {
			return i + 1
		}
	}
	return math.MaxInt
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
