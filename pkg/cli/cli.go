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
	"strconv"
	"strings"
	"syscall"

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
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/datastruct"
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

func (err ErrInvalidInput) ArgIndex() (int, bool) {
	return err.ref.ArgIndex()
}

func (err ErrInvalidInput) Error() string {
	return "[ErrInvalidInput]"
}

func (err ErrInvalidInput) Unwrap() error {
	return err.Err
}

///////////////////////////////////////////////////////////////////////////////////////////////////

type Handler interface {
	ServeCLI(w ResponseWriter, r *Request)
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

type Mux struct {
	m    map[string]*muxEntry
	path string
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
	Mux *Mux
}

func (m *Mux) Handle(pattern string, h Handler) {
	path := m.toPath(pattern)
	e := m.entryFor(path)
	if e.Handler != nil {
		panic(fmt.Sprintf("The %q pattern already had a handler registered", pattern))
	}
	e.Handler = h
}

func (m *Mux) Sub(pattern string) *Mux {
	path := m.toPath(pattern)
	e := m.entryFor(path)
	return e.Mux
}

func (m *Mux) ServeCLI(w ResponseWriter, r *Request) {
	if r == nil {
		w.ExitCode(1)
		return
	}
	if len(r.Args) == 0 {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)
		return
	}

	name, ok := slicekit.Shift(&r.Args)
	if !ok {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)
		return
	}

	{ // configure command name
		var (
			ctx        = r.Context()
			cmd string = name
		)
		if cmdName, ok := ctxCommandName.Lookup(ctx); ok {
			cmd = cmdName + " " + name
		}

		r = r.WithContext(ctxCommandName.ContextWith(ctx, cmd))
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

		m.helpUsage(o)
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
	}

	if entry.Handler == nil {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(o)
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

func (m *Mux) entryFor(path []string) *muxEntry {
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
			e.Mux = &Mux{}
		}
		mux = e.Mux
		ent = e
	}
	return ent
}

func (m Mux) toPath(pattern string) []string {
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
	if err := metaFor(ptr).Configure(r); err != nil {
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
	h = ptr.Elem().Interface().(Handler)
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

	/*
		if err := m.serveCLI(entry, w, r); err != nil {
			ServeCLI()
			isHelp := errors.Is(err, flag.ErrHelp)

			var o io.Writer = w
			if !isHelp {
				o = errOut(w)
			}

			if entry.Handler != nil {
				helpUsageOf(o, entry.Handler, m.getPath()+" "+name)
				m.helpLineBreak(o, 1)
			}

			if !isHelp {
				w.ExitCode(ExitCodeBadRequest)
				printfln(o, err.Error())
			}
		}
	*/
}

type stop struct{}

// func Stop() { panic(stop{}) }
//
// func HandleError(w Response, r *Request, err error) {
// 	if err == nil {
// 		return
// 	}
// 	w.ExitCode(ExitCodeError)
// 	fmt.Fprintf(w, "%s\n", err.Error())
// 	Stop()
// }

func NewStdRequest(ctx context.Context) *Request {
	var args []string
	if 1 < len(os.Args) {
		args = os.Args[1:]
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
	var (
		w = &StdResponse{}
		r = NewStdRequest(ctx)
	)

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

	ServeCLI(h, w, r)
	osint.Exit(w.Code)
}

func ConfigureHandler[H Handler](ptr *H, r *Request) error {
	return metaFor(reflect.ValueOf(ptr)).Configure(r)
}

func (m *Mux) entries() map[string]*muxEntry {
	if m.m == nil {
		m.m = map[string]*muxEntry{}
	}
	return m.m
}

func (m Mux) getPath() string {
	if m.path != "" {
		return m.path
	}
	return execName()
}

type ctxCommandNameKey struct{}

var ctxCommandName contextkit.ValueHandler[ctxCommandNameKey, string]

func commandName(ctx context.Context) string {
	if name, ok := ctxCommandName.Lookup(ctx); ok {
		return name
	}
	return execName()
}

func execName() string {
	if ep, err := os.Executable(); err == nil {
		return filepath.Base(ep)
	}
	if 0 < len(os.Args) {
		return os.Args[0]
	}
	return ""
}

func printfln(w io.Writer, msg ...string) {
	w.Write([]byte(strings.Join(msg, lineSeparator) + lineSeparator))
}

var lineSeparator = func() string {
	switch runtime.GOOS {
	case "windows":
		return "\r\n"
	default:
		return "\n"
	}
}()

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
func (rr *StdResponse) Stdeout() io.Writer                { return os.Stdout }
func (rr *StdResponse) Stderr() io.Writer                 { return os.Stderr }
func (rr *StdResponse) Write(p []byte) (n int, err error) { return rr.Stdeout().Write(p) }

///////////////////////////////////////////////////////////////////////////////////////////////////

type ResponseRecorder struct {
	Code int
	Out  bytes.Buffer
	Err  bytes.Buffer
}

func (rr *ResponseRecorder) ExitCode(n int)                    { rr.Code = n }
func (rr *ResponseRecorder) Stdeout() io.Writer                { return &rr.Out }
func (rr *ResponseRecorder) Stderr() (_ io.Writer)             { return &rr.Err }
func (rr *ResponseRecorder) Write(p []byte) (n int, err error) { return rr.Stdeout().Write(p) }

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

var argTagHandler = reflectkit.TagHandler[int]{
	Name: "arg",
	Parse: func(field reflect.StructField, tagName, tagValue string) (int, error) {
		return strconv.Atoi(tagValue)
	},
	Use: func(field reflect.StructField, value reflect.Value, index int) error {
		return nil
	},

	PanicOnParseError: true,
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

func metaFor(ptr reflect.Value) metaH {
	if ptr.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("unable to configure %s type due to receiving it as non-pointer type", ptr.Type().String()))
	}
	var m = metaH{ptr: ptr}
	for v := range reflectkit.Visit(ptr) {
		if v.Type != reftree.StructField {
			continue
		}
		if !v.StructField.IsExported() {
			continue
		}
		var ref = &metaRef{Node: v}
		if ref.IsRelevant() {
			m.refs = append(m.refs, ref)
		}
	}
	return m
}

type metaH struct {
	ptr  reflect.Value
	refs []*metaRef
}

func (m metaH) Validate(ctx context.Context) (rErr error) {
	defer errorkit.Recover(&rErr)
	for i, a := range m.Args() {
		if a.Index != i {
			const format = "%s field is an arg, and it was expected to be at index %d but it has the index of %d"
			return fmt.Errorf(format, a.Ref.FieldName(), i, a.Index)
		}
	}
	var ufn datastruct.Set[string]
	for _, flag := range m.Flags() {
		for _, name := range flag.Names {
			if ufn.Contains(name) {
				return fmt.Errorf("flag collision on %s", name)
			}
			ufn.Append(name)
		}
	}
	return nil
}

func (m metaH) Configure(r *Request) error {
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

	for _, a := range slicekit.IterReverse(m.Args()) {
		raw, ok := slicekit.PopAt(&r.Args, a.Index)
		if ok {
			a.Ref.IsSet = true
			a.Ref.Raw = raw
		}
	}

	var ctx = r.Context()
	for _, ref := range m.refs {
		if err := ref.Parse(ctx); err != nil {
			return err
		}
	}
	return nil
	// TODO: maybe validate the whole structure too?
	//
	// if validate.Value(ctx, m.ptr.Interface()) != nil {
	// 	// some error but difficult to tell what
	// 	// maybe integrate error handler
	// return errInternalValidationError
	// }
}

type metaArg struct {
	Ref   *metaRef
	Index int
	Name  string
}

func (m metaH) Args() []metaArg {
	var args []metaArg
	for _, ref := range m.refs {
		index, ok := ref.ArgIndex()
		if !ok {
			continue
		}
		args = append(args, metaArg{
			Ref:   ref,
			Index: index,
			Name:  ref.Node.StructField.Name,
		})
	}
	slicekit.SortBy(args, func(a, b metaArg) bool {
		return a.Index < b.Index
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
	Node  reftree.Node
	IsSet bool
	Raw   string
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

func (ref metaRef) Parse(ctx context.Context) (rErr error) {
	defer errorkit.FinishOnError(&rErr, func() {
		rErr = ErrInvalidInput{
			ref: ref,
			Err: rErr,
		}
	})
	typ, ok := ref.LookupType()
	if !ok {
		return fmt.Errorf("unable to determine type for %s", ref.InputName())
	}
	if ref.IsSet {
		return ref.parseRaw(ctx, typ, ref.Raw)
	}
	if envVal, ok := ref.LookupEnv(); ok {
		return ref.parseRaw(ctx, typ, envVal)
	}
	if defVal, ok := ref.LookupDefault(); ok {
		return ref.parseRaw(ctx, typ, defVal)
	}
	var isEmpty = reflectkit.IsEmpty(ref.Node.Value)
	if !ref.IsRequired() && isEmpty {
		return nil
	}
	if isEmpty {
		return fmt.Errorf("missing input for %s", ref.InputName())
	}
	// switch ref.Node.Type {
	// case reftree.StructField:
	// 	return validate.StructField(ctx, ref.Node.StructField, ref.Node.Value)
	// }
	return nil
}

func (ref metaRef) parseRaw(ctx context.Context, typ reflect.Type, raw string) error {
	value, err := convkit.ParseReflect(typ, raw)
	if err != nil {
		return err
	}
	if ref.Node.Type == reftree.StructField {
		if err := validate.StructField(ctx, ref.Node.StructField, value); err != nil {
			// enum error is soncidered as a user error,
			// this we intercept the validation error
			// and return back the list of enum value which are accepted for this field.
			if errors.Is(err, enum.ErrInvalid) {
				return enumError(ref.InputName(), enum.ReflectValues(ref.Node.StructField), value)
			}
			return err
		}
	}

	ref.Node.Value.Set(value)
	return nil
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

func (ref metaRef) ArgIndex() (int, bool) {
	index, ok, _ := argTagHandler.Lookup(ref.Node.StructField)
	return index, ok
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

// func (sf flagMeta) Setter(ctx context.Context, Struct reflect.Value, value flagValue) (rErr error) {
// 	name := strings.Join(sf.Names, "/")
// 	defer errorkit.Recover(&rErr)
// 	field := Struct.FieldByIndex(sf.StructField.Index)

// 	field, ok := reflectkit.ToSettable(field)
// 	if !ok {
// 		if sf.StructField.Anonymous {
// 			return nil
// 		}
// 		const ErrNotSettableField errorkit.Error = "ErrNotSettableField"
// 		return ErrNotSettableField.F("%s field is not settable in %s", sf.StructField.Name, Struct.Type().String())
// 	}

// 	if !value.IsSet && sf.HasDefault { // use default value
// 		field.Set(sf.DefVal)
// 		return nil
// 	}
// 	if !value.IsSet {
// 		if !reflectkit.IsZero(field) {
// 			return nil
// 		}
// 		if sf.HasDefault {
// 			field.Set(sf.DefVal)
// 			return nil
// 		}
// 		if sf.Required { // raise error for the missing but expected flag
// 			return ErrFlagMissing.F("%s flag is required", name)
// 		}
// 		return nil // ignore the flag, no value to be dependency injected
// 	}
// 	rval, err := convkit.ParseReflect(field.Type(), value.Raw)
// 	if err != nil {
// 		return ErrFlagParseIssue.F("%s (%s) encountered a parsing error with the value of: %q", name, field.Type().String(), value.Raw)
// 	}

// 	if err := validate.StructField(ctx, sf.StructField, rval); err != nil {
// 		if errors.Is(err, enum.ErrInvalid) {
// 			return ErrFlagInvalid.F("%w", enumError(name, sf.Enum, rval))
// 		}
// 	}

// 	field.Set(rval)
// 	return nil
// }

// func (sf structFlag) Setter(Struct reflect.Value, value flagValue) (rErr error) {
// 	name := strings.Join(sf.Names, "/")
// 	defer errorkit.Recover(&rErr)
// 	field := Struct.FieldByIndex(sf.StructField.Index)
// 	if len(raw) == 0 && sf.HasDefault { // use default value
// 		field.Set(sf.DefVal)
// 		return nil
// 	}
// 	if len(raw) == 0 {
// 		if sf.Required { // raise error for the missing but expected flag
// 			return ErrFlagMissing.F("%s flag is required", name)
// 		}
// 		return nil // ignore the flag, no value to be dependency injected
// 	}
// 	rval, err := convkit.ParseReflect(field.Type(), raw)
// 	if err != nil {
// 		return ErrFlagParseIssue.F("%s (%s) encountered a parsing error with the value of: %q", name, field.Type().String(), raw)
// 	}
// 	if 0 < len(sf.Enum) {
// 		_, ok := slicekit.Find(sf.Enum, func(e reflect.Value) bool {
// 			return reflectkit.Equal(e, rval)
// 		})
// 		if !ok {
// 			return ErrFlagInvalid.F("%s got the value of %s which is not part of the acceptable values", name, rval.Interface())
// 		}
// 	}
// 	field.Set(rval)
// 	return nil
// }

// func (sf flagMeta) mapToFlagSet(ctx context.Context, fs *flag.FlagSet, Struct reflect.Value) func() error {
// 	var v = flagValue{Type: sf.StructField.Type}
// 	for _, n := range sf.Names {
// 		fs.Var(&v, n, sf.Default)
// 	}
// 	return func() error { return sf.Setter(ctx, Struct, v) }
// }

type flagValue struct {
	Ref *metaRef

	Raw     string
	Default string
	IsSet   bool
	Type    reflect.Type
}

func (v *flagValue) String() string { return v.Raw }

func (v *flagValue) IsBoolFlag() bool {
	if typ, ok := v.Ref.LookupType(); ok {
		return typ.Kind() == reflect.Bool
	}
	return false
}

func (v *flagValue) Set(raw string) error {
	v.Raw = raw
	v.IsSet = true
	return nil
}

func (v *flagValue) Link() {
	if v.IsSet {
		v.Ref.IsSet = true
		v.Ref.Raw = v.Raw
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
