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
	"go.llib.dev/frameless/port/ds/dsset"
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

func (err ErrInvalidInput) ArgTag() (argTag, bool) {
	return err.ref.ArgTag()
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
	m map[string]*muxEntry
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
		m.helpUsage(r.Context(), o)
		m.helpLineBreak(o, 1)
		m.helpCommands(o)
		return
	}

	name, ok := slicekit.Shift(&r.Args)
	if !ok {
		w.ExitCode(2)
		o := errOut(w)
		m.helpUsage(r.Context(), o)
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
		m.helpUsage(r.Context(), o)
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
	if err := mh.Validate(r.Context()); err != nil {
		w.ExitCode(ExitCodeBadRequest)
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
}

type stop struct{}

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

type ctxCommandNameKey struct{}

var ctxCommandName contextkit.ValueHandler[ctxCommandNameKey, string]

func commandName(ctx context.Context) string {
	cmd, _ := ctxCommandName.Lookup(ctx)
	return cmd
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

func (rr *ResponseRecorder) CombinedOutput() []byte {
	var out = make([]byte, 0, rr.Out.Len()+rr.Err.Len())
	out = append(out, rr.Out.Bytes()...)
	out = append(out, rr.Err.Bytes()...)
	return out
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

type argTag struct {
	Field      reflect.StructField
	IsVariadic bool
	Begin      *int
	End        *int
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
	for v := range reflectkit.Visit(ptr) {
		if v.Type != reftree.StructField {
			continue
		}
		if !v.StructField.IsExported() {
			continue
		}
		var ref = &metaRef{Node: v}
		if sep, ok, _ := separatorTagHandler.Lookup(ref.Node.StructField); ok {
			ref.Separator = sep
		}
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

	// Validate that range arguments have enough values when partially provided or fully expected
	for _, ref := range m.refs {
		if tag, ok := ref.ArgTag(); ok {
			var actualLen = len(ref.RawValues)
			switch {
			case tag.Begin != nil && tag.End != nil:
				expectedLen := *tag.End - *tag.Begin
				// If any values were provided but not enough, it's an error (partial provision)
				if 0 < actualLen && actualLen < expectedLen {
					return fmt.Errorf("too few arguments for %s: expected range [%d:%d] (%d values), got %d", ref.Node.StructField.Name, *tag.Begin, *tag.End, expectedLen, actualLen)
				}
				// If no values were provided and it's required, it's an error
				if actualLen == 0 && ref.IsRequired() {
					return fmt.Errorf("missing arguments for %s: expected range [%d:%d]", ref.InputName(), *tag.Begin, *tag.End)
				}
			case tag.Begin != nil && tag.End == nil && tag.IsVariadic:
				// If no values were provided and it's required, it's an error
				if actualLen == 0 && ref.IsRequired() {
					return fmt.Errorf("missing arguments for %s: expected range [%d:]", ref.InputName(), *tag.Begin)
				}
			}
		}
	}

	var ufn dsset.Set[string]
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

func (ref metaRef) parseVariadicRaw(ctx context.Context, typ reflect.Type) error {
	var opts []convkit.Option
	if ref.Separator != "" {
		opts = append(opts, convkit.Options{Separator: ref.Separator})
	}

	switch typ.Kind() {
	case reflect.Slice:
		sliceVal := reflect.MakeSlice(typ, 0, len(ref.RawValues))
		for _, raw := range ref.RawValues {
			elemVal, err := convkit.ParseReflect(typ.Elem(), raw, opts...)
			if err != nil {
				return err
			}
			sliceVal = reflect.Append(sliceVal, elemVal)
		}
		ref.Node.Value.Set(sliceVal)
		// Validate the field after setting the value
		if ref.Node.Type == reftree.StructField {
			if err := validate.StructField(ctx, ref.Node.StructField, sliceVal); err != nil {
				if errors.Is(err, enum.ErrInvalid) {
					return enumError(ref.InputName(), enum.ReflectValues(ref.Node.StructField), sliceVal)
				}
				return err
			}
		}
		return nil
	default:
		// For non-slice types with variadic tag, this should have been caught by validation
		if len(ref.RawValues) > 0 {
			return ref.parseRaw(ctx, typ, ref.RawValues[0])
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
	// Handle variadic arguments with multiple raw values
	if len(ref.RawValues) > 0 {
		return ref.parseVariadicRaw(ctx, typ)
	}

	// Handle variadic arguments with no values - create empty slice and validate
	if tag, ok := ref.ArgTag(); ok && tag.IsVariadic && len(ref.RawValues) == 0 {
		if typ.Kind() == reflect.Slice {
			sliceVal := reflect.MakeSlice(typ, 0, 0)
			ref.Node.Value.Set(sliceVal)
			// Validate the empty slice against any length constraints
			if ref.Node.Type == reftree.StructField {
				if err := validate.StructField(ctx, ref.Node.StructField, sliceVal); err != nil {
					if errors.Is(err, enum.ErrInvalid) {
						return enumError(ref.InputName(), enum.ReflectValues(ref.Node.StructField), sliceVal)
					}
					return err
				}
			}
			return nil
		}
	}

	// Handle range arguments with no values - check if required
	if tag, ok := ref.ArgTag(); ok && !tag.IsVariadic && tag.Begin != nil && tag.End != nil {
		actualLen := len(ref.RawValues)
		if actualLen == 0 {
			if ref.IsRequired() {
				return fmt.Errorf("missing arguments for %s: expected range [%d:%d]", ref.InputName(), *tag.Begin, *tag.End)
			}
			// Optional range with no args - set empty slice
			if typ.Kind() == reflect.Slice {
				sliceVal := reflect.MakeSlice(typ, 0, 0)
				ref.Node.Value.Set(sliceVal)
				return nil
			}
		}
		// Note: partial provision (actualLen > 0 && actualLen < expectedLen) is handled in Validate()
		// If we have some values but not enough, we still need to set what we got for the field
		if typ.Kind() == reflect.Slice {
			sliceVal := reflect.MakeSlice(typ, 0, len(ref.RawValues))
			for _, raw := range ref.RawValues {
				elemVal, err := convkit.ParseReflect(typ.Elem(), raw)
				if err != nil {
					return err
				}
				sliceVal = reflect.Append(sliceVal, elemVal)
			}
			ref.Node.Value.Set(sliceVal)
		}
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
	var opts []convkit.Option
	if ref.Separator != "" {
		opts = append(opts, convkit.Options{Separator: ref.Separator})
	}

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
				partVal, err := convkit.ParseReflect(typ, part, opts...)
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
			value, err = convkit.ParseReflect(typ, parts[0], opts...)
			if err != nil {
				return err
			}
		}
	} else {
		var err error
		value, err = convkit.ParseReflect(typ, raw, opts...)
		if err != nil {
			return err
		}
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
