package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/internal/interr"
	"go.llib.dev/frameless/internal/sandbox"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/internal/signalint"
	"go.llib.dev/frameless/pkg/internal/tagint"
	"go.llib.dev/frameless/pkg/internal/validateint"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/validate"
)

const (
	// ExitCodeOK : Success
	ExitCodeOK = 0
	// ExitCodeError : General Error
	ExitCodeError = 1
	// ExitCodeBadRequest : Misuse of shell builtins or invalid command-line usage, often equated with a bad request.
	ExitCodeBadRequest = 2
)

const (
	ErrFlagMissing    errorkitlite.Error = "ErrFlagMissing"
	ErrFlagParseIssue errorkitlite.Error = "ErrFlagParseIssue"
	ErrFlagInvalid    errorkitlite.Error = "ErrFlagInvalid"

	ErrArgMissing      errorkitlite.Error = "ErrArgMissing"
	ErrArgParseIssue   errorkitlite.Error = "ErrArgParseIssue"
	ErrArgIndexInvalid errorkitlite.Error = "ErrArgIndexInvalid"

	ErrInvalidDefaultValue errorkitlite.Error = "ErrInvalidDefaultValue"
)

// ErrInvalidInput is the error of an input that is not valid,
// like a missing required flag, an arg that can't be parsed, or a validate.Validatable value that is not valid.
// Err holds the cause, without its stack trace, as the error is about the input of the user, not about the code.
type ErrInvalidInput struct {
	Err error

	ref metaRef
}

func (err ErrInvalidInput) ValidateError() (validate.Error, bool) {
	return errorkit.As[validate.Error](err.Err)
}

func (err ErrInvalidInput) Flags() []string {
	return err.ref.Flags()
}

func (err ErrInvalidInput) Envs() []string {
	return err.ref.Envs()
}

func (err ErrInvalidInput) ArgTag() (argTag, bool) {
	return err.ref.ArgTag()
}

// Error tells which input is not valid, the way the user gives it, and why.
func (err ErrInvalidInput) Error() string {
	var msg = "[ErrInvalidInput]"
	if name, ok := err.inputName(); ok {
		msg += " " + name
	}
	if err.Err != nil {
		msg += ": " + err.Err.Error()
	}
	return msg
}

// inputName names the invalid input as a flag, an arg position, or an env var.
// A field which is not an input, like a validate.Validatable dependency, is named by its field name.
// An error of the handler itself has no name.
func (err ErrInvalidInput) inputName() (string, bool) {
	if err.ref.Node.Type != reftree.StructField {
		return "", false
	}
	if name, ok := slicekit.First(err.ref.Flags()); ok {
		return "-" + name, true
	}
	if tag, ok := err.ref.ArgTag(); ok && tag.Begin != nil {
		return "arg" + tag.PositionExpr(), true
	}
	if name, ok := slicekit.First(err.ref.Envs()); ok {
		return "$" + name, true
	}
	return err.ref.FieldName(), true
}

func (err ErrInvalidInput) Unwrap() error {
	return err.Err
}

///////////////////////////////////////////////////////////////////////////////////////////////////

type Handler interface {
	ServeCLI(w ResponseWriter, r *Request)
}

// Middleware is an interface that a field of a Handler can implement,
// typically a configuration struct, to wrap the Handler with additional behaviour.
//
// Once the inputs (flags, args, env vars, defaults) are loaded,
// ServeCLI and Build look for Middleware fields in the Handler recursively,
// and wrap the Handler with them.
// They run in the order of their field declaration,
// and an outer field runs before the Middleware fields it holds.
// So with the fields Foo, Bar, Baz, the execution order is Foo -> Bar -> Baz -> Handler.
//
// Each Middleware runs once, even when more than one field holds it, like a shared pointer.
// A Middleware embedded into a struct field runs through that field, when the field is a Middleware itself,
// either through the promoted MiddlewareCLI method, or through its own one, which shadows it, like in Go.
// The Handler is not a Middleware of itself, so a Middleware that the Handler embeds runs as any other field.
//
// MiddlewareCLI is called on the configured field value.
// A pointer receiver implementation is supported as well, even when the field is not a pointer,
// in which case MiddlewareCLI is called on the field itself.
// This allows a Middleware to set up the field for the next Handlers,
// like making a client from the configuration in the returned Handler,
// and the Handler sees it, even when its ServeCLI has a value receiver.
//
// Like any Handler, the returned Handler owns the reply.
// To fail the request, reply with cli.Error and don't call the next Handler.
// Work that can fail, such as connecting to a dependency with the configured values,
// belongs in the returned Handler, where the request and the response are at hand.
// Returning a nil Handler is a programming error, which makes ServeCLI and Build panic.
type Middleware interface {
	MiddlewareCLI(next Handler) Handler
}

type Request struct {
	Args []string
	Body io.Reader

	ctx context.Context
}

type ResponseWriter interface {
	ExitCode(n int)
	io.Writer
}

type ErrorWriter interface {
	Stderr() io.Writer
}

func (r Request) Context() context.Context {
	if r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

func (r *Request) WithContext(ctx context.Context) *Request {
	if ctx == nil {
		panic("nil context")
	}
	r2 := new(Request)
	*r2 = *r
	r2.ctx = ctx
	return r2
}

type HandlerFunc func(w ResponseWriter, r *Request)

func (fn HandlerFunc) ServeCLI(w ResponseWriter, r *Request) {
	fn(w, r)
}

///////////////////////////////////////////////////////////////////////////////////////////////////

// ServeMux is a cli command multiplexer.
type ServeMux struct {
	m map[string]*muxEntry

	summary     string
	description string
}

func (m *ServeMux) WithSummary(d string) *ServeMux {
	m.summary = d
	return m
}

func (m *ServeMux) WithDescription(d string) *ServeMux {
	m.description = d
	return m
}

var _ HelpSummary = (*ServeMux)(nil)

// Summary returns the description of this ServeMux, implementing HelpSummary.
func (m *ServeMux) Summary() string {
	return m.summary
}

var _ HelpDescription = (*ServeMux)(nil)

// Description returns the description of this ServeMux, implementing HelpDescription.
func (m *ServeMux) Description() string {
	return m.description
}

func (m *ServeMux) helpSummaryDescription(w io.Writer) {
	var hasSummary = 0 < len(m.summary)
	if hasSummary {
		printfln(w, m.summary)
	}
	if 0 < len(m.description) {
		if hasSummary {
			m.helpLineBreak(w, 1)
		}
		printfln(w, m.description)
	}
}

// Multiplexer is an interface that, when implemented by a command,
// delegates the parsing of input arguments and options to the Handler in cli.Main.
//
// If you want to create your own Mux, simply implement this interface in your structure.
type Multiplexer interface {
	Handle(pattern string, h Handler)
}

type muxEntry struct {
	Handler Handler
	// meta    *meta
	Mux *ServeMux
}

func (m *ServeMux) Handle(pattern string, h Handler) {
	path := m.toPath(pattern)
	if len(path) == 0 {
		panic("pattern cannot be empty")
	}
	e := m.entryFor(path)
	if e.Handler != nil {
		panic(fmt.Sprintf("The %q pattern already had a handler registered", pattern))
	}
	e.Handler = h
}

func (m *ServeMux) Sub(pattern string) *ServeMux {
	path := m.toPath(pattern)
	if len(path) == 0 {
		panic("pattern cannot be empty")
	}
	e := m.entryFor(path)
	return e.Mux
}

func (m *ServeMux) ServeCLI(w ResponseWriter, r *Request) {
	if r == nil {
		w.ExitCode(1)
		return
	}
	if len(r.Args) == 0 {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(r.Context(), o)
		m.helpSummaryDescription(o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)
		return
	}

	name, ok := slicekit.Shift(&r.Args)
	if !ok {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(r.Context(), o)
		m.helpSummaryDescription(o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)
		return
	}

	{ // configure command name
		var (
			ctx        = r.Context()
			cmd string = name
		)
		if !isHelpFlag(name) {
			if cmdName, ok := ctxCommandName.Lookup(ctx); ok {
				cmd = cmdName + " " + name
			}

			r = r.WithContext(ctxCommandName.ContextWith(ctx, cmd))
		}
	}

	entry, ok := m.entries()[name]
	if !ok {
		isHelp := isHelpFlag(name)
		if !isHelp {
			w.ExitCode(2)
		}

		var o io.Writer = w
		if !isHelp {
			o = errOut(w)
		}

		m.helpUsage(r.Context(), o)
		m.helpSummaryDescription(o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)

		if !isHelp {
			printfln(o, "command is unknown: "+name, "")
		}

		return
	}

	if entry.Mux != nil && 0 < len(r.Args) {
		next := r.Args[0]
		if _, ok := entry.Mux.entries()[next]; ok {
			entry.Mux.ServeCLI(w, r)
			return
		}
		// If any remaining arg is a help flag and there's no Handler, recurse into sub-mux to show its help
		if entry.Handler == nil {
			if slices.ContainsFunc(r.Args, isHelpFlag) {
				entry.Mux.ServeCLI(w, r)
				return
			}
		}
	}

	if entry.Handler == nil {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(r.Context(), o)
		m.helpSummaryDescription(o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)
		return
	}

	ServeCLI(entry.Handler, w, r)
}

func isHelpFlag(v string) bool {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	err := fs.Parse([]string{v})
	return errors.Is(err, flag.ErrHelp)
}

func (m *ServeMux) entryFor(path []string) *muxEntry {
	var (
		ent *muxEntry
		mux = m
	)
	for _, name := range path {
		e, ok := mux.entries()[name]
		if !ok {
			e = &muxEntry{}
			mux.entries()[name] = e
		}
		if e.Mux == nil {
			e.Mux = &ServeMux{}
		}
		mux = e.Mux
		ent = e
	}
	return ent
}

func (m ServeMux) toPath(pattern string) []string {
	return strings.Fields(pattern)
}

func ServeCLI(h Handler, w ResponseWriter, r *Request) {
	if h == nil {
		panic("nil cli.Handler")
	}
	if w == nil {
		panic("nil cli.Response")
	}
	if r == nil {
		panic("nil *cli.Request")
	}
	if _, ok := h.(Multiplexer); ok {
		// When the Handler is also a multiplexer,
		// we leave the configuration to it
		// so it can handle routing properly.
		h.ServeCLI(w, r)
		return
	}

	ptr := reflect.New(reflect.TypeOf(h))
	ptr.Elem().Set(reflect.ValueOf(h))
	var mh = metaFor(ptr)
	if err := mh.Configure(r); err != nil {
		var exitCode = ExitCodeBadRequest
		if isHelp(err) {
			exitCode = ExitCodeOK
		}
		w.ExitCode(exitCode)
		var msg = toHelp(r.Context(), h, err)
		var o io.Writer = w
		if !isHelp(err) {
			o = errOut(w)
		}
		printfln(o, msg)
		return
	}
	h = mh.Handler()
	o := sandbox.Run(func() {
		h.ServeCLI(w, r)
	})
	if o.OK {
		return
	}
	if o.Goexit {
		runtime.Goexit()
	}
	if o.Panic {
		if _, ok := o.PanicValue.(stop); !ok {
			panic(o.PanicValue)
		}
	}
}

type stop struct{}

func NewStdRequest(ctx context.Context) *Request {
	var args []string
	if vs := osint.Args(); 1 < len(vs) {
		args = vs[1:]
	}
	return &Request{
		ctx:  ctx,
		Args: args,
		Body: os.Stdin,
	}
}

func Main(ctx context.Context, h Handler) {
	logger.Configure(func(l *logging.Logger) {
		if l.Out == nil { // avoid logging into STDOUT as a CLI app
			l.Out = os.Stderr
		}
	})

	sigch := make(chan os.Signal)
	signals := []os.Signal{
		syscall.SIGINT,
		syscall.SIGHUP,
		syscall.SIGTERM,
	}

	signalint.Notify(sigch, signals...)
	defer close(sigch)
	defer signalint.Stop(sigch)

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-sigch:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	var (
		w = &StdResponse{}
		r = NewStdRequest(ctx)
	)

	ServeCLI(h, w, r)
	osint.Exit(w.Code)
}

// Build configures the handler with the inputs from the request,
// the same way as ServeCLI would, and then wraps it with the Middleware fields it has.
// The configured handler is set back to ptr as well,
// while the returned Handler is the one wrapped with the Middlewares.
// The returned Handler serves with the handler behind ptr at the time of the call,
// so the changes that pointer receiver Middlewares make on its fields are visible to it.
//
// Once the inputs are loaded, the validate.Validatable values are validated:
// the fields of the handler recursively, inner fields first, and then the handler itself.
// An empty input field (flag, arg, env) is left out,
// so an optional input that was not given is not rejected for its zero value.
// Each Middleware and Validatable is used once, even when more than one field holds it.
// An embedded one is used through the struct that embeds it, when that struct is a Middleware or Validatable itself,
// the same way as Go promotes or shadows the method of an embedded field.
//
// The returned error is mostly about the inputs of the request,
// like an invalid flag value, a missing required argument,
// or a Validatable value which is not valid with the given inputs.
// An arg definition that can't take inputs, like a gap between arg positions, is returned as an error too,
// regardless of the inputs.
// Other programming errors, like two input fields with the same flag name, make Build panic.
func Build[H Handler](ptr *H, r *Request) (Handler, error) {
	if ptr == nil {
		panic("nil cli.Handler pointer")
	}
	mh := metaFor(reflect.ValueOf(ptr))
	if err := mh.Configure(r); err != nil {
		return nil, err
	}
	return mh.Handler(), nil
}

func (m *ServeMux) entries() map[string]*muxEntry {
	if m.m == nil {
		m.m = map[string]*muxEntry{}
	}
	return m.m
}

type ctxCommandNameKey struct{}

var ctxCommandName contextkit.ValueHandler[ctxCommandNameKey, string]

func commandName(ctx context.Context) string {
	cmd, _ := ctxCommandName.Lookup(ctx)
	return cmd
}

func execName() string {
	if ep, err := osint.Executable(); err == nil {
		return trimExeExt(filepath.Base(ep))
	}
	if 0 < len(osint.Args()) {
		return trimExeExt(osint.Args()[0])
	}
	return ""
}

// trimExeExt drops the Windows ".exe" extension from the program name,
// so the usage reads the same on every OS, and matches how users invoke the program.
func trimExeExt(name string) string {
	if ext := filepath.Ext(name); strings.EqualFold(ext, ".exe") {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

func printfln(w io.Writer, msg ...string) {
	w.Write([]byte(strings.Join(msg, lineSeparator) + lineSeparator))
}

// lineSeparator is used for all framework generated text, regardless of the OS,
// the same way as the flag and fmt packages do.
// This keeps the output consistent with what handlers write using fmt.Fprintln,
// and makes the output identical on every platform.
const lineSeparator = "\n"

func errOut(w ResponseWriter) io.Writer {
	if rwe, ok := w.(ErrorWriter); ok {
		if o := rwe.Stderr(); o != nil {
			return o
		}
	}
	return w
}

type StdResponse struct{ Code int }

func (rr *StdResponse) ExitCode(n int)                    { rr.Code = n }
func (rr *StdResponse) Stdout() io.Writer                 { return os.Stdout }
func (rr *StdResponse) Stderr() io.Writer                 { return os.Stderr }
func (rr *StdResponse) Write(p []byte) (n int, err error) { return rr.Stdout().Write(p) }

///////////////////////////////////////////////////////////////////////////////////////////////////

type ResponseRecorder struct {
	Code int
	Out  bytes.Buffer
	Err  bytes.Buffer
}

func (rr *ResponseRecorder) CombinedOutput() []byte {
	var out = make([]byte, 0, rr.Out.Len()+rr.Err.Len())
	out = append(out, rr.Out.Bytes()...)
	out = append(out, rr.Err.Bytes()...)
	return out
}

func (rr *ResponseRecorder) ExitCode(n int)                    { rr.Code = n }
func (rr *ResponseRecorder) Stdout() io.Writer                 { return &rr.Out }
func (rr *ResponseRecorder) Stderr() (_ io.Writer)             { return &rr.Err }
func (rr *ResponseRecorder) Write(p []byte) (n int, err error) { return rr.Stdout().Write(p) }

var flagTagHandler = reflectkit.TagHandler[[]string]{
	Name: "flag",
	Parse: func(field reflect.StructField, tagName, tagValue string) ([]string, error) {
		var names = splitFlagTag(tagValue)
		if len(names) == 0 {
			return nil, fmt.Errorf("%s's flag tag is empty", field.Name)
		}
		for _, name := range names {
			if err := validateFlagName(name); err != nil {
				return nil, err
			}
		}
		return names, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, names []string) error {
		return nil
	},

	PanicOnParseError: true,
}

func splitFlagTag(tag string) []string {
	flags := strings.Split(tag, ",")
	flags = slicekit.Map(flags, func(flag string) string {
		flag = strings.TrimSpace(flag)
		for strings.HasPrefix(flag, "-") {
			flag = strings.TrimPrefix(flag, "-")
		}
		return flag
	})
	flags = slicekit.Filter(flags, func(flag string) bool {
		return 0 < len(flag)
	})
	return flags
}

func validateFlagName(name string) (rErr error) {
	defer errorkit.Recover(&rErr)
	var fs flag.FlagSet
	fs.Var((*nullFlagValue)(nil), name, "")
	return nil
}

type nullFlagValue struct{}

func (*nullFlagValue) String() string   { return "" }
func (*nullFlagValue) Set(string) error { return nil }

type argTag struct {
	Field      reflect.StructField
	IsVariadic bool
	Begin      *int
	End        *int
}

// IsSlice reports whether the arg takes multiple values, like a range ("n:m") or a variadic ("n:") arg.
func (tag argTag) IsSlice() bool {
	return tag.Begin != nil && (tag.IsVariadic || tag.End != nil)
}

// RangeExpr formats the slice expression of a range or variadic arg, such as "[1:3]" or "[1:]".
func (tag argTag) RangeExpr() string {
	if tag.Begin == nil {
		return "[:]"
	}
	if tag.End == nil {
		return fmt.Sprintf("[%d:]", *tag.Begin)
	}
	return fmt.Sprintf("[%d:%d]", *tag.Begin, *tag.End)
}

// PositionExpr formats the position of the arg as an index expression,
// such as "[0]" for a single position, "[1:3]" for a range, or "[1:]" for a variadic arg.
func (tag argTag) PositionExpr() string {
	if tag.Begin != nil && !tag.IsSlice() {
		return fmt.Sprintf("[%d]", *tag.Begin)
	}
	return tag.RangeExpr()
}

var argTagHandler = reflectkit.TagHandler[argTag]{
	Name:  "arg",
	Parse: parseArgTag,
	Use:   useArgTag,

	PanicOnParseError: true,
	ForceCache:        true,
}

// parseArgTag parses an arg tag value and returns an argTag struct.
// Supported formats:
//   - "n" - single argument at index n (e.g., "0", "1", "2")
//   - "n:" - variadic arguments starting from index n to end (e.g., "1:", "2:")
//   - ":" - all arguments (variadic, equivalent to "0:")
//   - "n:m" - range from index n (inclusive) to m (exclusive), like Go slicing (e.g., "1:3")
func parseArgTag(field reflect.StructField, tagName, tagValue string) (argTag, error) {
	var result argTag
	result.Field = field

	if !strings.Contains(tagValue, ":") {
		// Single index like "0", "1", etc.
		idx, err := strconv.Atoi(tagValue)
		if err != nil {
			return argTag{}, fmt.Errorf("invalid arg index %q: %w", tagValue, err)
		}
		result.Begin = &idx
		// result.End = &idx
		result.IsVariadic = false
		return result, nil
	}

	// Handle ":" meaning all arguments
	if tagValue == ":" {
		result.Begin = new(int) // [:] == [0:]
		result.IsVariadic = true
		return result, nil
	}

	var parts = strings.Split(tagValue, ":")
	if len(parts) != 2 {
		return argTag{}, fmt.Errorf("invalid arg tag format %q", tagValue)
	}

	// by default both ":" and ":m" starts from index zero
	result.Begin = new(int) // 0

	// if n in "n:m" is provided
	if beginRaw := parts[0]; 0 < len(beginRaw) {
		idx, err := strconv.Atoi(beginRaw)
		if err != nil {
			return argTag{}, fmt.Errorf("invalid begin index %q: %w", beginRaw, err)
		}
		result.Begin = &idx
	}

	if endRaw := parts[1]; 0 < len(endRaw) {
		idx, err := strconv.Atoi(endRaw)
		if err != nil {
			return argTag{}, fmt.Errorf("invalid end index %q: %w", parts[1], err)
		}
		result.End = &idx
		// This is a fixed range like "0:5", not variadic
		result.IsVariadic = false
	} else {
		// Only variadic if no end is specified (e.g., "n:" or ":")
		result.IsVariadic = true
	}

	return result, nil
}

func useArgTag(field reflect.StructField, value reflect.Value, v argTag) error {
	// Validate that variadic tags are only used with slice types - panic on error
	if v.IsVariadic && field.Type.Kind() != reflect.Slice {
		panic(fmt.Sprintf(`variadic arg tag selector with ":" only supported for []T types, got %s`, field.Type.String()))
	}
	return nil
}

var descTagHandler = reflectkit.TagHandler[string]{
	Name:  "desc",
	Alias: []string{"description"},

	Parse: func(field reflect.StructField, tagName, tagValue string) (string, error) {
		return tagValue, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, v string) error {
		return nil
	},
}

// TODO: maybe this should be in validation

var isRequiredTagHandler = reflectkit.TagHandler[bool]{
	Name: "required",
	Parse: func(field reflect.StructField, tagName, tagValue string) (bool, error) {
		if len(tagValue) == 0 {
			return true, nil
		}
		return convkit.Parse[bool](tagValue)
	},
	Use: func(field reflect.StructField, value reflect.Value, v bool) error {
		return nil
	},
	PanicOnParseError: true,
}

var isOptionalTagHandler = reflectkit.TagHandler[bool]{
	Name:  "optional",
	Alias: []string{"opt"},
	Parse: func(field reflect.StructField, tagName, tagValue string) (bool, error) {
		if len(tagValue) == 0 {
			return true, nil
		}
		return convkit.Parse[bool](tagValue)
	},
	Use: func(field reflect.StructField, value reflect.Value, v bool) error {
		return nil
	},
	PanicOnParseError: true,
}

var separatorTagHandler = reflectkit.TagHandler[string]{
	Name:  "separator",
	Alias: []string{"sep"},
	Parse: func(field reflect.StructField, tagName, tagValue string) (string, error) {
		return tagValue, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, v string) error {
		return nil
	},
}

func metaFor(ptr reflect.Value) metaH {
	if ptr.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("unable to configure %s type due to receiving it as non-pointer type", ptr.Type().String()))
	}
	var m = metaH{ptr: ptr}
	m.walkFields(func(field reftree.Node) bool {
		var ref = &metaRef{Node: field}
		if sep, ok, _ := separatorTagHandler.Lookup(ref.Node.StructField); ok {
			ref.Separator = sep
		}
		if ref.IsRelevant() {
			m.refs = append(m.refs, ref)
		}
		return true
	})
	return m
}

type metaH struct {
	ptr  reflect.Value
	refs []*metaRef
}

// walkFields visits the exported fields of the handler recursively,
// in field declaration order, where an outer field precedes its inner fields.
// The fields of a visited field are looked into only when visit returns true.
// An opaque field is visited, as it can be an input itself, but its value is not looked into.
func (m metaH) walkFields(visit func(field reftree.Node) (stepIn bool)) {
	_ = reftree.Walk(m.ptr, func(v reftree.Node) error {
		if v.Type != reftree.StructField {
			return nil
		}
		if v.StructField.IsExported() && !visit(v) {
			return reftree.Skip
		}
		if tagint.IsOpaque(v.StructField) {
			return reftree.Skip
		}
		return nil
	})
}

// asInterface returns the value as T, when the value implements T, or its address does.
// A nil value doesn't count as a T.
func asInterface[T any](v reflect.Value) (T, bool) {
	var zero T
	if !v.IsValid() || !v.CanInterface() {
		return zero, false
	}
	var typ = reflectkit.TypeOf[T]()
	if v.CanAddr() && v.Addr().Type().Implements(typ) {
		return v.Addr().Interface().(T), true
	}
	if !v.Type().Implements(typ) {
		return zero, false
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		if v.IsNil() {
			return zero, false
		}
	}
	return v.Interface().(T), true
}

// middlewares collects the Middleware fields of the handler,
// in field declaration order, where an outer field precedes its inner fields.
// The handler is not a Middleware of itself, so the Middleware fields it embeds are collected.
func (m metaH) middlewares() []Middleware {
	var mws []Middleware
	for _, ref := range fieldsOf[Middleware](m, false, nil) {
		mw, _ := asInterface[Middleware](ref.Node.Value)
		mws = append(mws, mw)
	}
	return mws
}

// fieldsOf collects the fields of the handler which implement T,
// in field declaration order, where an outer field precedes its inner fields.
//
// Each T is collected once, so it is used once:
//   - an embedded field is left out when the struct field that embeds it is a T itself,
//     either through the method promoted from the embedded field, or through its own method, which shadows it, like in Go;
//   - a T behind a pointer, which more than one field holds, is collected through the first field only.
//
// handlerIsT tells whether the handler itself is used as a T,
// in which case the fields that the handler embeds are left out the same way.
// A field for which skip returns true is left out, along with the fields it holds.
func fieldsOf[T any](m metaH, handlerIsT bool, skip func(field reftree.Node) bool) []*metaRef {
	var (
		refs []*metaRef
		seen = make(map[any]struct{})
	)
	m.walkFields(func(field reftree.Node) bool {
		if skip != nil && skip(field) {
			return false
		}
		v, ok := asInterface[T](field.Value)
		if !ok || isPromoted[T](field, handlerIsT) {
			return true
		}
		if key, ok := pointerIdentity(v); ok {
			if _, dup := seen[key]; dup {
				return true
			}
			seen[key] = struct{}{}
		}
		refs = append(refs, &metaRef{Node: field})
		return true
	})
	return refs
}

// isPromoted tells whether the field is embedded into a struct field which is a T itself,
// so the T of the field is used through that struct field instead.
// When the handler embeds the field, it is the handler that the field is promoted to, if handlerIsT.
func isPromoted[T any](field reftree.Node, handlerIsT bool) bool {
	for field.StructField.Anonymous {
		holder := field.Parent
		for holder != nil && holder.Type != reftree.StructField {
			switch holder.Type {
			case reftree.Struct, reftree.Pointer, reftree.PointerElem, reftree.Interface, reftree.InterfaceElem:
				holder = holder.Parent
			default: // held by a slice, an array or a map, which don't promote methods
				return false
			}
		}
		if holder == nil { // embedded into the handler
			return handlerIsT
		}
		if _, ok := asInterface[T](holder.Value); ok && holder.StructField.IsExported() {
			return true
		}
		field = *holder
	}
	return false
}

// pointerIdentity returns v as an identity, when v is a pointer, which is the same for every field that holds it.
// A pointer to a zero sized value is not an identity, since such values can share their address.
func pointerIdentity(v any) (any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Type().Elem().Size() == 0 {
		return nil, false
	}
	return v, true
}

// refOf returns the input which the field is.
// Every walk of the handler makes new nodes, so the field is matched to an input by its address,
// and by its type, which tells apart an embedded struct from its first field that shares its address.
// A field which is not addressable can't be set, so as an input, it is one that was not loaded.
func (m metaH) refOf(field reftree.Node) (*metaRef, bool) {
	if !field.Value.CanAddr() {
		ref := &metaRef{Node: field}
		return ref, ref.IsRelevant()
	}
	for _, ref := range m.refs {
		if v := ref.Node.Value; v.CanAddr() && v.Type() == field.Value.Type() && v.Addr().Pointer() == field.Value.Addr().Pointer() {
			return ref, true
		}
	}
	return nil, false
}

// Handler returns the handler behind m.ptr, wrapped with the handler's Middleware fields.
func (m metaH) Handler() Handler {
	var h Handler = HandlerFunc(m.serveCLI)
	mws := m.middlewares()
	for i := len(mws) - 1; 0 <= i; i-- {
		next := mws[i].MiddlewareCLI(h)
		if next == nil {
			panic(fmt.Sprintf("%T middleware returned a nil cli.Handler", mws[i]))
		}
		h = next
	}
	return h
}

// serveCLI serves with the handler that m.ptr points to at the time of the call, not at the time of the build,
// so the changes that pointer receiver Middlewares make on its fields are visible to it,
// even when its ServeCLI has a value receiver.
func (m metaH) serveCLI(w ResponseWriter, r *Request) {
	m.ptr.Elem().Interface().(Handler).ServeCLI(w, r)
}

// checkArgPositions checks that the args of the handler are defined at consecutive positions.
func (m metaH) checkArgPositions() error {
	// Only validate single-index args for sequential ordering
	// Slice args (variadic/range) don't need strict index validation
	var expectedIdx int
	for _, a := range m.Args() {
		if !a.Variadic && a.End == nil && a.Begin != nil {
			// Single index argument - check it's at the expected position
			if *a.Begin != expectedIdx {
				const format = "%s field is an arg, and it was expected to be at index %d but it has the index of %d"
				return fmt.Errorf(format, a.Ref.FieldName(), expectedIdx, *a.Begin)
			}
			expectedIdx++
		} else if a.Begin != nil {
			// Slice argument - just ensure begin index is not before current position
			if *a.Begin < expectedIdx {
				const format = "%s field's arg range starts at %d but previous args already consumed up to index %d"
				return fmt.Errorf(format, a.Ref.FieldName(), *a.Begin, expectedIdx)
			}
			expectedIdx = *a.Begin + 1
		}
	}
	return nil
}

// checkFlagCollisions panics when more than one input field of the handler defines the same flag name,
// like two fields of the same config type, as it is a programming error.
func (m metaH) checkFlagCollisions() {
	var defined = make(map[string]*metaRef)
	for _, flg := range m.Flags() {
		for _, name := range flg.Names {
			if other, ok := defined[name]; ok {
				panic(fmt.Sprintf("flag collision on -%s: %s and %s", name, other.FieldPath(), flg.Ref.FieldPath()))
			}
			defined[name] = flg.Ref
		}
	}
}

// checkArgRanges checks that a range arg got every value of its range, when it got any.
// A range or variadic arg without any user value is handled by metaH.load,
// since it may still get its value from the environment, its default tag, or the handler itself.
func (m metaH) checkArgRanges() error {
	for _, ref := range m.refs {
		tag, ok := ref.ArgTag()
		if !ok || tag.Begin == nil || tag.End == nil {
			continue
		}
		var (
			actualLen   = len(ref.RawValues)
			expectedLen = *tag.End - *tag.Begin
		)
		if 0 < actualLen && actualLen < expectedLen {
			return fmt.Errorf("too few arguments for %s: expected range [%d:%d] (%d values), got %d", ref.Node.StructField.Name, *tag.Begin, *tag.End, expectedLen, actualLen)
		}
	}
	return nil
}

// Configure loads the inputs of the request into the handler.
// The definition of the handler is checked first, so its errors come regardless of the inputs.
func (m metaH) Configure(r *Request) error {
	if err := m.checkArgPositions(); err != nil {
		return err
	}
	m.checkFlagCollisions()

	var flagSetOutput bytes.Buffer
	var flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	flagSet.Usage = func() {}
	flagSet.SetOutput(&flagSetOutput)

	var flagValues []*flagValue
	for _, flg := range m.Flags() {
		var fv = &flagValue{Ref: flg.Ref}
		for _, n := range flg.Names {
			flagSet.Var(fv, n, "")
		}
		flagValues = append(flagValues, fv)
	}

	if err := flagSet.Parse(r.Args); err != nil {
		return err
	}

	r.Args = flagSet.Args()

	for _, fv := range flagValues {
		fv.Link()
	}

	// Validate that variadic arg tags are only used with slice types.
	// This is a programming error that should panic immediately.
	for _, ref := range m.refs {
		if tag, ok := ref.ArgTag(); ok && tag.IsVariadic {
			typ, ok := ref.LookupType()
			if !ok {
				continue
			}
			if typ.Kind() != reflect.Slice {
				panic(fmt.Sprintf(`variadic arg tag selector with ":" only supported for []T types, got %s`, typ.String()))
			}
		}
	}

	// Process arguments: capture all values based on original indices first.
	// We process in order of begin index to handle overlapping ranges correctly.
	var consumedIndices = make(map[int]bool)

	// First pass: identify which indices each arg wants to consume
	type argRange struct {
		a       metaArg
		start   int
		end     int
		isSlice bool
	}
	var allRanges []argRange

	for _, a := range m.Args() {
		if a.Begin == nil {
			continue
		}
		start := *a.Begin
		end := len(r.Args)
		isSlice := false

		if a.Variadic || (a.End != nil && !func() bool { return *a.End <= start }()) {
			isSlice = true
			if a.End != nil {
				end = *a.End
				if end > len(r.Args) {
					end = len(r.Args)
				}
			}
		}

		allRanges = append(allRanges, argRange{a: a, start: start, end: end, isSlice: isSlice})
	}

	// Sort by start index to process in order
	slicekit.SortBy(allRanges, func(a, b argRange) bool {
		return a.start < b.start
	})

	// Second pass: assign values, skipping indices already consumed
	for _, ar := range allRanges {
		if ar.isSlice {
			// For slice args, collect unconsumed values in range
			var rawValues []string
			for i := ar.start; i < ar.end && i < len(r.Args); i++ {
				if !consumedIndices[i] {
					rawValues = append(rawValues, r.Args[i])
					consumedIndices[i] = true
				}
			}
			ar.a.Ref.RawValues = rawValues
			if len(rawValues) > 0 {
				ar.a.Ref.IsSet = true
			}
		} else {
			// For single args, take the value if not consumed
			if ar.start < len(r.Args) && !consumedIndices[ar.start] {
				consumedIndices[ar.start] = true
				ar.a.Ref.IsSet = true
				ar.a.Ref.Raw = r.Args[ar.start]
			}
		}
	}

	// Clean up r.Args to remove all consumed indices
	var remainingArgs []string
	for i, arg := range r.Args {
		if !consumedIndices[i] {
			remainingArgs = append(remainingArgs, arg)
		}
	}
	r.Args = remainingArgs

	if err := m.checkArgRanges(); err != nil {
		return err
	}
	return m.load(r.Context())
}

// load sets the input fields from their sources, in the order of their precedence,
// so a later source overwrites an earlier one:
//
//  1. the `default` tag values, outer fields first, and only onto zero fields,
//     so the default of an inner field only fills in what the default of its struct field left zero;
//  2. the flags, args and env vars, inner fields first,
//     so the JSON input of a struct field is merged onto the inputs of its inner fields.
//
// So a default never overwrites an input, and the order of the flags in the arguments doesn't matter.
// The required inputs and the validation tags are checked only once every value is loaded,
// since an inner field can get its value from the JSON value of its struct field as well.
// The validate.Validatable values of the handler are validated last, see checkValidatables,
// so the validation tag checks leave them out, as each must be validated only once.
func (m metaH) load(ctx context.Context) error {
	for _, ref := range m.refs {
		if err := ref.loadDefault(); err != nil {
			return err
		}
	}
	for _, ref := range m.innerFirst() {
		if err := ref.loadInput(); err != nil {
			return err
		}
	}
	for _, ref := range m.refs {
		if err := ref.check(validateint.SkipValidatable(ctx)); err != nil {
			return err
		}
	}
	return m.checkValidatables(ctx)
}

// checkValidatables validates the validate.Validatable fields of the handler recursively,
// where an inner field is validated before the struct field that holds it,
// and then the handler itself.
// Each Validatable is validated once, see fieldsOf.
//
// An input which was not given, so it is empty, is left out, along with the fields it holds,
// so an optional input is not rejected for its zero value.
// An input given with an empty value is validated,
// and so is an input with a value, even when it got the value through its inner inputs.
func (m metaH) checkValidatables(ctx context.Context) error {
	handler, handlerIsValidatable := asInterface[validate.Validatable](m.ptr.Elem())
	refs := fieldsOf[validate.Validatable](m, handlerIsValidatable, func(field reftree.Node) bool {
		ref, ok := m.refOf(field)
		return ok && !ref.loaded && reflectkit.IsEmpty(field.Value)
	})
	for _, ref := range innerFirst(refs) {
		v, _ := asInterface[validate.Validatable](ref.Node.Value)
		if err := callValidate(ctx, v); err != nil {
			return ErrInvalidInput{ref: *ref, Err: errorkit.WithoutTrace(err)}
		}
	}
	if handlerIsValidatable {
		if err := callValidate(ctx, handler); err != nil {
			return ErrInvalidInput{Err: errorkit.WithoutTrace(err)}
		}
	}
	return nil
}

// callValidate returns the error of Validate as a validate.Error, unless it is one already.
func callValidate(ctx context.Context, v validate.Validatable) error {
	err := v.Validate(ctx)
	if err == nil {
		return nil
	}
	if _, ok := err.(validate.Error); ok {
		return err
	}
	return validate.Error{Cause: err}
}

// innerFirst returns the refs in an order where every inner field precedes the struct field that holds it.
func (m metaH) innerFirst() []*metaRef {
	return innerFirst(m.refs)
}

func innerFirst(refs []*metaRef) []*metaRef {
	refs = slices.Clone(refs)
	slices.SortStableFunc(refs, func(a, b *metaRef) int {
		return b.depth() - a.depth()
	})
	return refs
}

func (ref metaRef) parseVariadicRaw(typ reflect.Type) error {
	switch typ.Kind() {
	case reflect.Slice:
		sliceVal := reflect.MakeSlice(typ, 0, len(ref.RawValues))
		for _, raw := range ref.RawValues {
			elemVal, err := ref.parseValue(typ.Elem(), raw)
			if err != nil {
				return err
			}
			sliceVal = reflect.Append(sliceVal, elemVal)
		}
		ref.Node.Value.Set(sliceVal)
		return nil
	default:
		// For non-slice types with variadic tag, this should have been caught by validation
		if len(ref.RawValues) > 0 {
			return ref.parseRaw(typ, ref.RawValues[0])
		}
		return nil
	}
}

type metaArg struct {
	Ref      *metaRef
	Begin    *int
	End      *int
	Variadic bool
	Name     string
}

func (m metaH) Args() []metaArg {
	var args []metaArg
	for _, ref := range m.refs {
		tag, ok := ref.ArgTag()
		if !ok {
			continue
		}
		args = append(args, metaArg{
			Ref:      ref,
			Begin:    tag.Begin,
			End:      tag.End,
			Variadic: tag.IsVariadic,
			Name:     ref.Node.StructField.Name,
		})
	}
	slicekit.SortBy(args, func(a, b metaArg) bool {
		var aIdx, bIdx int
		if a.Begin != nil {
			aIdx = *a.Begin
		}
		if b.Begin != nil {
			bIdx = *b.Begin
		}
		return aIdx < bIdx
	})
	return args
}

type metaFlag struct {
	Ref *metaRef

	Names []string
}

func (m metaH) Flags() []metaFlag {
	var mFlags []metaFlag
	for _, ref := range m.refs {
		if !ref.IsFlag() {
			continue
		}
		names := ref.Flags()
		mFlags = append(mFlags, metaFlag{
			Ref:   ref,
			Names: names,
		})
	}
	return mFlags
}

type metaRef struct {
	Node      reftree.Node
	IsSet     bool
	Raw       string
	RawValues []string
	Separator string

	// loaded tells whether the field got a value of its own, from its input or its `default` tag.
	loaded bool
}

func (ref metaRef) InputName() string {
	if ref.IsFlag() {
		fname, _ := slicekit.First(ref.Flags())
		return fname
	}
	if ref.IsArg() {
		return ref.FieldName()
	}
	if ref.IsEnv() {
		ename, _ := slicekit.First(ref.Envs())
		return ename
	}
	return ref.FieldName()
}

func (ref metaRef) FieldName() string {
	if ref.Node.Parent != nil {
		return ref.Node.Parent.Value.Type().String() + "." + ref.Node.StructField.Name
	}
	return ref.Node.StructField.Name
}

// FieldPath is the path of the field from the handler, like pkg.MyCommand.Config.Host,
// which tells apart the fields of the same type at different places of the handler.
func (ref metaRef) FieldPath() string {
	var (
		root  string
		names []string
	)
	for n := range ref.Node.IterUpward() {
		switch n.Type {
		case reftree.StructField:
			names = append(names, n.StructField.Name)
		case reftree.Struct:
			root = n.Value.Type().String()
		}
	}
	slices.Reverse(names)
	return strings.Join(append([]string{root}, names...), ".")
}

// depth is the number of nodes between the field and the root of the handler.
func (ref metaRef) depth() int {
	var n int
	for p := ref.Node.Parent; p != nil; p = p.Parent {
		n++
	}
	return n
}

// HasInput tells whether the field got an input: a flag, an arg, or an env var.
func (ref metaRef) HasInput() bool {
	if len(ref.RawValues) > 0 || ref.IsSet {
		return true
	}
	_, ok := ref.LookupEnv()
	return ok
}

func (ref *metaRef) asInvalidInput(rErr *error) {
	errorkit.FinishOnError(rErr, func() {
		*rErr = ErrInvalidInput{
			ref: *ref,
			Err: errorkit.WithoutTrace(*rErr),
		}
	})
}

// loadDefault sets the field to its `default` tag value, unless the field has a value already.
// A field with an input skips it, since the input overwrites it anyway,
// except a field taking a JSON object, since its JSON input is merged onto the default.
func (ref *metaRef) loadDefault() (rErr error) {
	defer ref.asInvalidInput(&rErr)
	typ, ok := ref.LookupType()
	if !ok {
		return fmt.Errorf("unable to determine type for %s", ref.InputName())
	}
	if ref.HasInput() && !tagint.IsJSONObject(typ) {
		return nil
	}
	return ref.useDefault(typ)
}

// loadInput sets the field from its input, if it got one.
func (ref *metaRef) loadInput() (rErr error) {
	defer ref.asInvalidInput(&rErr)
	typ, ok := ref.LookupType()
	if !ok {
		return fmt.Errorf("unable to determine type for %s", ref.InputName())
	}
	var err error
	switch {
	case len(ref.RawValues) > 0:
		// Range and variadic args with values from the user.
		// Partially provided ranges are rejected by metaH.Validate.
		err = ref.parseVariadicRaw(typ)
	case ref.IsSet:
		err = ref.parseRaw(typ, ref.Raw)
	default:
		envVal, ok := ref.LookupEnv()
		if !ok {
			return nil
		}
		err = ref.parseRaw(typ, envVal)
	}
	if err != nil {
		return err
	}
	ref.loaded = true
	return nil
}

// check validates the field's value when it got one of its own,
// and otherwise reports the field as missing when it is required, but its value is empty.
// A value from the JSON value of an outer struct field satisfies required as well,
// and so does a value from the inner fields of a struct field.
func (ref *metaRef) check(ctx context.Context) (rErr error) {
	defer ref.asInvalidInput(&rErr)
	if ref.loaded {
		return ref.validate(ctx, ref.Node.Value)
	}
	typ, ok := ref.LookupType()
	if !ok {
		return fmt.Errorf("unable to determine type for %s", ref.InputName())
	}
	var isEmpty = reflectkit.IsEmpty(ref.Node.Value)
	if tag, ok := ref.ArgTag(); ok && tag.IsSlice() && isEmpty {
		return ref.parseEmptySliceArg(ctx, typ, tag)
	}
	if isEmpty && ref.IsRequired() {
		return fmt.Errorf("missing input for %s", ref.InputName())
	}
	return nil
}

// parseEmptySliceArg handles a range or variadic arg that got no value from the user,
// the environment, its `default` tag, or the handler itself.
// A required one is a missing input, an optional one becomes an empty slice.
func (ref metaRef) parseEmptySliceArg(ctx context.Context, typ reflect.Type, tag argTag) error {
	if ref.IsRequired() {
		return fmt.Errorf("missing arguments for %s: expected range %s", ref.InputName(), tag.RangeExpr())
	}
	if typ.Kind() != reflect.Slice {
		return nil
	}
	sliceVal := reflect.MakeSlice(typ, 0, 0)
	ref.Node.Value.Set(sliceVal)
	// Validate the empty slice against any length constraints
	return ref.validate(ctx, sliceVal)
}

// validate checks the value against the validation tags of the field, such as `enum` or `len`.
func (ref metaRef) validate(ctx context.Context, value reflect.Value) error {
	if ref.Node.Type != reftree.StructField {
		return nil
	}
	if err := validate.StructField(ctx, ref.Node.StructField, value); err != nil {
		// enum error is considered as a user error,
		// thus we intercept the validation error
		// and return back the list of enum values which are accepted for this field.
		if errors.Is(err, enum.ErrInvalid) {
			return enumError(ref.InputName(), enum.ReflectValues(ref.Node.StructField), value)
		}
		return err
	}
	return nil
}

// useDefault sets the field to its `default` tag value when the field is still zero.
// A non-zero field holds a value pre-set on the handler, or one from the default of its struct field,
// which must not be overwritten.
func (ref *metaRef) useDefault(typ reflect.Type) error {
	defVal, ok := ref.LookupDefault()
	if !ok || !reflectkit.IsZero(ref.Node.Value) {
		return nil
	}
	if err := ref.parseRaw(typ, defVal); err != nil {
		return err
	}
	ref.loaded = true
	return nil
}

// parseRaw parses the raw input value, and sets the field to it.
// The value is validated only once every input field is loaded, see metaRef.check.
func (ref metaRef) parseRaw(typ reflect.Type, raw string) error {
	// Handle multiple flag repetitions by splitting on the internal separator first.
	// This allows both `-v foo,bar -v baz` and `-vs foo/bar/baz` to work correctly.
	var value reflect.Value
	if strings.Contains(raw, flagValueSeparator) {
		// Multiple flag values were provided via repetition.
		// Parse each part separately and combine them into a slice.
		parts := strings.Split(raw, flagValueSeparator)
		typ, ok := ref.LookupType()
		if !ok {
			return fmt.Errorf("unable to determine type for %s", ref.InputName())
		}

		switch typ.Kind() {
		case reflect.Slice:
			sliceVal := reflect.MakeSlice(typ, 0, 0)
			for _, part := range parts {
				partVal, err := ref.parseValue(typ, part)
				if err != nil {
					return err
				}
				// Append each element from the parsed part to our result slice.
				for i := 0; i < partVal.Len(); i++ {
					sliceVal = reflect.Append(sliceVal, partVal.Index(i))
				}
			}
			value = sliceVal
		default:
			// For non-slice types, just use the first value.
			var err error
			value, err = ref.parseValue(typ, parts[0])
			if err != nil {
				return err
			}
		}
	} else {
		var err error
		value, err = ref.parseValue(typ, raw)
		if err != nil {
			return err
		}
	}

	ref.Node.Value.Set(value)
	return nil
}

// parseValue parses a raw input value as the given type.
//
// A slice value is split with the `separator` tag of the field, or with tagint.DefaultSeparator when it has none.
// A JSON array is parsed as JSON, which allows passing list elements that contain the separator.
// A JSON object for the field itself is merged onto the field's current value.
// The env package follows the same rules.
func (ref metaRef) parseValue(typ reflect.Type, raw string) (reflect.Value, error) {
	opts := convkit.Options{
		Separator:  tagint.Separator(ref.Separator, raw),
		TimeLayout: ref.TimeLayout(),
	}
	if typ == ref.Node.Value.Type() {
		return tagint.ParseOnto(ref.Node.Value, raw, opts)
	}
	return convkit.ParseReflect(typ, raw, opts)
}

// TimeLayout is the layout used to parse a time.Time input.
// It comes from the `layout` tag, and without one, the input is expected in RFC3339 format.
func (ref metaRef) TimeLayout() string {
	if layout, ok, _ := tagint.TimeLayout.Lookup(ref.Node.StructField); ok && layout != "" {
		return layout
	}
	return time.RFC3339
}

var typeTime = reflectkit.TypeOf[time.Time]()

// IsTime reports whether the input is a time.Time, or a pointer or slice of it.
func (ref metaRef) IsTime() bool {
	typ, ok := ref.LookupType()
	if !ok {
		return false
	}
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	return typ == typeTime
}

func (ref metaRef) LookupType() (reflect.Type, bool) {
	if ref.Node.StructField.Type != nil {
		return ref.Node.StructField.Type, true
	}
	if ref.Node.Value.IsValid() {
		return ref.Node.Value.Type(), true
	}
	return nil, false
}

func (ref metaRef) LookupDescription() (string, bool) {
	_, desc, ok := descTagHandler.LookupTag(ref.Node.StructField)
	return desc, ok
}

func (ref metaRef) LookupDefault() (string, bool) {
	_, val, ok := defaultTag.LookupTag(ref.Node.StructField)
	return val, ok
}

type InitDefaultTagValue func() (reflect.Value, error)

var defaultTag = reflectkit.TagHandler[InitDefaultTagValue]{
	Name: "default",
	Parse: func(sf reflect.StructField, tagName, tagValue string) (InitDefaultTagValue, error) {
		if reflectkit.IsMutableType(sf.Type) {
			return func() (reflect.Value, error) { return parseDefaultValue(sf, tagValue) }, nil
		}
		val, err := parseDefaultValue(sf, tagValue)
		if err != nil {
			return nil, err
		}
		return func() (reflect.Value, error) { return val, nil }, nil
	},
	Use: func(sf reflect.StructField, field reflect.Value, getDefault InitDefaultTagValue) error {
		if !reflectkit.IsZero(field) {
			return nil
		}
		val, err := getDefault()
		if err != nil {
			return err
		}
		field, ok := reflectkit.ToSettable(field)
		if !ok { // unsettable values are ignored
			return nil
		}
		field.Set(val)
		return nil
	},
	ForceCache: true,
}

func parseDefaultValue(sf reflect.StructField, raw string) (reflect.Value, error) {
	val, err := convkit.ParseReflect(sf.Type, raw)
	if err != nil {
		const format = "%s field's default value is not a valid %s type: %w"
		return val, interr.ImplementationError.F(format, sf.Name, sf.Type, err)
	}
	return val, nil
}

// ArgTag returns the parsed argTag for this reference.
func (ref metaRef) ArgTag() (argTag, bool) {
	tag, ok, _ := argTagHandler.Lookup(ref.Node.StructField)
	if !ok {
		return argTag{}, false
	}
	return tag, true
}

// ArgIndex returns the begin index for this argument.
func (ref metaRef) ArgIndex() (int, bool) {
	tag, ok := ref.ArgTag()
	if !ok || tag.Begin == nil {
		return 0, false
	}
	return *tag.Begin, true
}

func (ref metaRef) Flags() []string {
	names, ok, err := flagTagHandler.Lookup(ref.Node.StructField)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return names
}

func (ref metaRef) IsFlag() bool {
	_, _, ok := flagTagHandler.LookupTag(ref.Node.StructField)
	return ok
}

func (ref metaRef) IsArg() bool {
	_, _, ok := argTagHandler.LookupTag(ref.Node.StructField)
	return ok
}

func (ref metaRef) Type() reflect.Type {
	if ref.Node.Type == reftree.StructField {
		return ref.Node.StructField.Type
	}
	return ref.Node.Value.Type()
}

func (ref metaRef) Envs() []string {
	names, _ := env.LookupFieldEnvNames(ref.Node.StructField)
	return names
}

func (ref metaRef) LookupEnv() (string, bool) {
	names, ok := env.LookupFieldEnvNames(ref.Node.StructField)
	if !ok {
		return "", false
	}
	for _, name := range names {
		if val, ok := os.LookupEnv(name); ok {
			return val, true
		}
	}
	return "", false
}

func (ref metaRef) IsRequired() bool {
	if _, ok := ref.LookupDefault(); ok {
		return false
	}
	if is, ok, _ := isRequiredTagHandler.Lookup(ref.Node.StructField); ok && is {
		return true
	}
	if is, ok, _ := isOptionalTagHandler.Lookup(ref.Node.StructField); ok && is {
		return false
	}
	if !ref.IsFlag() && (ref.IsArg() || ref.IsEnv()) {
		return true
	}
	return false
}

func (ref metaRef) IsEnv() bool {
	_, ok := env.LookupFieldEnvNames(ref.Node.StructField)
	return ok
}

func (ref metaRef) IsRelevant() bool {
	return ref.IsFlag() || ref.IsArg() || ref.IsEnv()
}

type envMeta struct {
	V reftree.Node
}

type flagMeta struct {
	V reftree.Node

	Default    string
	HasDefault bool
	DefVal     reflect.Value

	Names []string
	Desc  string

	Required bool
	Enum     []reflect.Value
}

// flagValueSeparator is the delimiter used to join multiple flag values.
// It's chosen to be unlikely to appear in normal command-line arguments.
const flagValueSeparator = "\x00"

type flagValue struct {
	Ref *metaRef

	Raw       string
	RawValues []string
	Default   string
	IsSet     bool
	Type      reflect.Type
}

func (v *flagValue) String() string { return v.Raw }

func (v *flagValue) IsBoolFlag() bool {
	if typ, ok := v.Ref.LookupType(); ok {
		return typ.Kind() == reflect.Bool
	}
	return false
}

func (v *flagValue) Set(raw string) error {
	v.RawValues = append(v.RawValues, raw)
	v.IsSet = true
	return nil
}

func (v *flagValue) Link() {
	if v.IsSet && len(v.RawValues) > 0 {
		v.Ref.IsSet = true
		v.Ref.Raw = strings.Join(v.RawValues, flagValueSeparator)
	}
}

var EnumError = errorkit.UserError{
	Code:    "enum-error",
	Message: "invalid enumeration value",
}

func enumError(name string, enums []reflect.Value, val reflect.Value) error {
	var acceptedValues []string
	for _, val := range enums {
		fval, err := convkit.Format[any](val.Interface())
		if err != nil {
			return err
		}
		acceptedValues = append(acceptedValues, fval)
	}
	acceptedValuesFormatted := strings.Join(slicekit.Map(acceptedValues, func(v string) string { return " - " + v }), lineSeparator)
	const format = "%s got the value of %v which is not part of the acceptable values\n\naccepted values:\n%s"
	return EnumError.Wrap((fmt.Errorf(format, name, val.Interface(), acceptedValuesFormatted)))
}

// Error replies to the request with the specified error message and exit code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func Error(w ResponseWriter, message string, code int) {
	w.ExitCode(code)
	if ew, ok := w.(ErrorWriter); ok {
		_, _ = fmt.Fprintln(ew.Stderr(), message)
		return
	}
	_, _ = fmt.Fprintln(w, message)
}
