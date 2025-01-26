package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
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
	ErrFlagMissing    errorkit.Error = "ErrFlagMissing"
	ErrFlagParseIssue errorkit.Error = "ErrFlagParseIssue"
	ErrFlagInvalid    errorkit.Error = "ErrFlagInvalid"

	ErrArgMissing      errorkit.Error = "ErrArgMissing"
	ErrArgParseIssue   errorkit.Error = "ErrArgParseIssue"
	ErrArgIndexInvalid errorkit.Error = "ErrArgIndexInvalid"

	ErrInvalidDefaultValue errorkit.Error = "ErrInvalidDefaultValue"
)

///////////////////////////////////////////////////////////////////////////////////////////////////

type Handler interface {
	ServeCLI(w Response, r *Request)
}

type Request struct {
	Args []string
	Body io.Reader

	ctx context.Context
}

type Response interface {
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

type HandlerFunc func(w Response, r *Request)

func (fn HandlerFunc) ServeCLI(w Response, r *Request) {
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
	meta    *structMeta
	Mux     *Mux
}

func (m *Mux) Handle(pattern string, h Handler) {
	path := m.toPath(pattern)
	e := m.entryFor(path)

	if e.Handler != nil {
		panic(fmt.Sprintf("The %q pattern already had a handler registered", pattern))
	}

	e.Handler = h

	sm, ok, err := structMetaFor(h)
	if err != nil {
		panic(err.Error())
	}
	if ok {
		e.meta = &sm
	}
}

func (m *Mux) Sub(pattern string) *Mux {
	path := m.toPath(pattern)
	e := m.entryFor(path)
	return e.Mux
}

func (m Mux) ServeCLI(w Response, r *Request) {
	if m.m == nil {
		return
	}
	if r == nil {
		w.ExitCode(1)
		return
	}
	if len(r.Args) == 0 {
		w.ExitCode(2)
		m.helpUsage(w)
		m.helpLineBreak(w, 1)
		m.helpCommands(w)
		return
	}

	name, ok := slicekit.Shift(&r.Args)
	if !ok {
		w.ExitCode(2)
		m.helpUsage(w)
		m.helpLineBreak(w, 1)
		m.helpCommands(w)
		return
	}

	entry, ok := m.entries()[name]
	if !ok {
		w.ExitCode(2)
		m.helpUsage(w)
		m.helpLineBreak(w, 1)
		printErrLn(w, "command is unknown: "+name, "")
		m.helpCommands(w)
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
		m.helpUsage(w)
		m.helpLineBreak(w, 1)
		m.helpCommands(w)
	}

	if err := m.serveCLI(entry, w, r); err != nil {
		w.ExitCode(ExitCodeBadRequest)
		helpUsageOf(w, entry.Handler, entry.meta, m.getPath()+" "+name)
		m.helpLineBreak(w, 1)
		printErrLn(w, err.Error())
	}
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

func (m Mux) serveCLI(e *muxEntry, w Response, r *Request) error {
	var h Handler = e.Handler

	if e.meta != nil {
		var err error
		h, err = configure(h, *e.meta, r)
		if err != nil {
			return err
		}
	}

	h.ServeCLI(w, r)
	return nil
}

func Main(ctx context.Context, h Handler) {
	var args []string
	if 1 < len(os.Args) {
		args = os.Args[1:]
	}

	r := &Request{
		ctx:  ctx,
		Args: args,
		Body: os.Stdin,
	}
	w := &stdResponse{}

	logger.Configure(func(l *logging.Logger) {
		l.Out = os.Stderr
	})

	if _, ok := h.(Multiplexer); !ok {
		handler, err := ConfigureHandler(h, execName(), r)
		if err != nil {
			usage, usageErr := Usage(h, execName())
			if usageErr != nil {
				panic(usageErr.Error())
			}
			printErrLn(w, usage)
			printErrLn(w)
			printErrLn(w, err.Error())
			osint.Exit(ExitCodeBadRequest)
		}
		h = handler
	}

	h.ServeCLI(w, r)
	osint.Exit(w.Code)
}

func ConfigureHandler(h Handler, path string, r *Request) (Handler, error) {
	sm, ok, err := structMetaFor(h)
	if err != nil {
		return nil, err
	}
	if !ok {
		return h, nil
	}
	handler, err := configure(h, sm, r)
	if err != nil {
		return nil, err
	}
	return handler, nil
}

type HelpUsage interface {
	Usage(pattern string) (string, error)
}

// Usage will generate a help usage message for a given handler on a given command request pattern/path.
func Usage(h Handler, pattern string) (string, error) {
	if u, ok := h.(HelpUsage); ok {
		return u.Usage(pattern)
	}

	var meta *structMeta
	m, ok, err := structMetaFor(h)
	if err != nil {
		return "", err
	}
	if ok {
		meta = &m
	}
	return helpCreateUsage(h, meta, pattern), nil
}

func configure(h Handler, meta structMeta, r *Request) (Handler, error) {
	ptr := reflect.New(reflect.TypeOf(h))
	handler := reflect.ValueOf(h)
	ptr.Elem().Set(handler)
	handler = ptr.Elem()
	val := reflectkit.BaseValue(ptr)

	var flagSetOutput bytes.Buffer
	var flagSet = flag.NewFlagSet("", flag.ContinueOnError)
	flagSet.Usage = func() {}
	flagSet.SetOutput(&flagSetOutput)

	var callbacks []func() error

	for _, f := range meta.Flags {
		f := f // scope variable
		cb := f.mapToFlagSet(flagSet, val)
		callbacks = append(callbacks, cb)
	}

	err := flagSet.Parse(r.Args)
	if err != nil {
		return h, err
	}

	r.Args = flagSet.Args()

	for _, cb := range callbacks {
		if err := cb(); err != nil {
			return h, err
		}
	}

	for _, a := range meta.Args {
		raw, ok := slicekit.Lookup(r.Args, a.Index)
		if err := a.Setter(val, raw, ok); err != nil {
			return h, err
		}
	}

	h = handler.Interface().(Handler) // dependency inject

	return h, nil
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

func execName() string {
	if ep, err := os.Executable(); err == nil {
		return filepath.Base(ep)
	}
	if 0 < len(os.Args) {
		return os.Args[0]
	}
	return ""
}

func (m Mux) helpLineBreak(w Response, n int) {
	for i := 0; i < n; i++ {
		errOut(w).Write([]byte(lineSeparator))
	}
}

func (m Mux) helpUsage(w Response) {
	var msg []string
	msg = append(msg, fmt.Sprintf("Usage: %s", m.getPath()))
	printErrLn(w, msg...)
}

func helpUsageOf(w Response, h Handler, meta *structMeta, path string) {
	printErrLn(w, helpCreateUsage(h, meta, path))
}

func helpCreateUsage(h Handler, meta *structMeta, path string) string {
	var lines []string

	var usage string
	usage += "Usage: " + path

	if meta != nil {
		if 0 < len(meta.Flags) {
			usage += " [OPTION]..."
		}
		if 0 < len(meta.Args) {
			for _, arg := range meta.Args {
				usage += fmt.Sprintf(" [%s]", arg.Name)
			}
		}
	}

	lines = append(lines, usage, "")

	if s, ok := h.(HelpSummary); ok {
		lines = append(lines, s.Summary(), "")
	}

	if meta != nil {
		if 0 < len(meta.Flags) {
			lines = append(lines, "Options:")
			for _, flag := range meta.Flags {
				name, ok := slicekit.First(flag.Names)
				if !ok {
					continue
				}

				line := fmt.Sprintf("  -%s=[%s]", name, flag.StructField.Type.String())
				if 0 < len(flag.Desc) {
					line += ": " + flag.Desc
				}
				if 0 < len(flag.Default) {
					line += fmt.Sprintf(" (Default: %s)", flag.Default)
				}

				lines = append(lines, line)

				for i := 1; i < len(flag.Names); i++ {
					lines = append(lines, fmt.Sprintf("  -%s", flag.Names[i]))
				}

			}
		}
		if 0 < len(meta.Args) {
			if 0 < len(meta.Flags) {
				lines = append(lines, "") // empty line for seperation
			}
			lines = append(lines, "Arguments:")
			for _, arg := range meta.Args {
				line := fmt.Sprintf("  %s [%s]", arg.Name, arg.StructField.Type.String())
				if 0 < len(arg.Desc) {
					line += ": " + arg.Desc
				}
				if 0 < len(arg.Default) {
					line += fmt.Sprintf(" (Default: %s)", arg.Default)
				}

				lines = append(lines, line)
			}
		}
	}

	return strings.Join(lines, lineSeparator)
}

func (m Mux) helpCommands(w Response) {
	var msg []string

	var cmds []string
	for name, entry := range m.entries() {
		var line string
		line = " - " + name

		if h, ok := entry.Handler.(HelpSummary); ok {
			line += ": " + h.Summary()
		}

		cmds = append(cmds, line)
	}

	if 0 < len(cmds) {
		msg = append(msg, "Commands:")
		sort.Strings(cmds)
		msg = append(msg, cmds...)
	}

	printErrLn(w, msg...)
}

var lineSeparator = func() string {
	switch runtime.GOOS {
	case "windows":
		return "\r\n"
	default:
		return "\n"
	}
}()

func printErrLn(w Response, msg ...string) {
	errOut(w).Write([]byte(strings.Join(msg, lineSeparator) + lineSeparator))
}

func errOut(w Response) io.Writer {
	if rwe, ok := w.(ErrorWriter); ok {
		if o := rwe.Stderr(); o != nil {
			return o
		}
	}
	return w
}

type stdResponse struct {
	Code int
	Err  *os.File
	Out  *os.File
}

func (rr *stdResponse) ExitCode(n int)                    { rr.Code = n }
func (rr *stdResponse) Stdeout() io.Writer                { return os.Stdout }
func (rr *stdResponse) Stderr() io.Writer                 { return os.Stderr }
func (rr *stdResponse) Write(p []byte) (n int, err error) { return rr.Stdeout().Write(p) }

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

func structMetaFor(h Handler) (structMeta, bool, error) {
	var (
		v = reflectkit.BaseValueOf(h)
		T = v.Type()
		m = structMeta{}
	)

	if v.Kind() != reflect.Struct {
		return m, false, nil
	}

	fieldNum := T.NumField()

	var foundFlags = map[string]struct{}{}

	for i := 0; i < fieldNum; i++ {
		sf := T.Field(i)

		sFlag, ok, err := scanForFlag(sf)
		if err != nil {
			return structMeta{}, false, err
		}
		if ok {
			for _, f := range sFlag.Names {
				foundFlags[f] = struct{}{}
			}
			m.Flags = append(m.Flags, sFlag)
		}
		sArg, ok, err := scanForArg(sf)
		if err != nil {
			return structMeta{}, false, err
		}
		if ok {
			for _, f := range sFlag.Names {
				foundFlags[f] = struct{}{}
			}
			m.Args = append(m.Args, sArg)
		}
	}

	slicekit.SortBy(m.Args, func(a, b structArg) bool {
		return a.Index < b.Index
	})

	for i, a := range m.Args {
		if a.Index != i {
			const format = "%s field is an arg, and it was expected to be at index %d but it has the index of %d"
			panic(fmt.Sprintf(format, a.Name, i, a.Index))
		}
	}

	return m, true, nil
}

func scanForFlag(sf reflect.StructField) (structFlag, bool, error) {
	flag, ok := sf.Tag.Lookup("flag")
	if !ok {
		return structFlag{}, false, nil
	}

	flags := splitFlag(flag)
	def, defVal, hasDefault, err := getDefault(sf)
	if err != nil {
		return structFlag{}, true, err
	}

	desc, _ := getDescription(sf)
	isRequired, err := getIsRequired(sf)
	if err != nil {
		return structFlag{}, true, err
	}

	enumValues, err := enum.ReflectValuesOfStructField(sf)
	if err != nil {
		return structFlag{}, true, err
	}

	return structFlag{
		StructField: sf,

		Default:    def,
		HasDefault: hasDefault,
		DefVal:     defVal,

		Names: flags,
		Desc:  desc,

		Required: isRequired,
		Enum:     enumValues,
	}, true, nil
}

func scanForArg(sf reflect.StructField) (structArg, bool, error) {
	argIndex, ok := sf.Tag.Lookup("arg")
	if !ok {
		return structArg{}, false, nil
	}

	index, err := strconv.Atoi(argIndex)
	if err != nil {
		panic(ErrArgIndexInvalid.F("invalid arg index for %s field: %q", sf.Name, argIndex))
	}

	def, defVal, hasDefault, err := getDefault(sf)
	if err != nil {
		return structArg{}, false, err
	}

	desc, _ := getDescription(sf)

	isRequired, err := getIsRequired(sf)
	if err != nil {
		return structArg{}, true, err
	}

	enumvs, err := enum.ReflectValuesOfStructField(sf)
	if err != nil {
		return structArg{}, true, err
	}

	return structArg{
		StructField: sf,

		Default:    def,
		DefVal:     defVal,
		HasDefault: hasDefault,

		Name:     sf.Name,
		Index:    index,
		Desc:     desc,
		Required: isRequired,
		Enum:     enumvs,
	}, true, nil
}

func getDefault(sf reflect.StructField) (string, reflect.Value, bool, error) {
	def, ok := sf.Tag.Lookup("default")
	if !ok {
		return "", reflect.New(sf.Type).Elem(), false, nil
	}
	val, err := convkit.ParseReflect(sf.Type, def)
	if err != nil {
		return "", reflect.Value{}, true, ErrInvalidDefaultValue.F("%s field got %q as default value, but it is not interpretable as %s", sf.Name, def, sf.Type.String())
	}
	return def, val, true, nil
}

var structDescriptionTags = []string{"desc", "description"}

func getDescription(sf reflect.StructField) (string, bool) {
	for _, tag := range structDescriptionTags {
		if v, ok := sf.Tag.Lookup(tag); ok {
			return v, true
		}
	}
	return "", false
}

func getIsRequired(sf reflect.StructField) (bool, error) {
	if _, _, ok, err := getDefault(sf); err == nil && ok {
		return false, nil
	}
	if req, ok := sf.Tag.Lookup("required"); ok {
		return strconv.ParseBool(req)
	}
	return false, nil
}

func splitFlag(flag string) []string {
	flags := strings.Split(flag, ",")
	flags = slicekit.Map(flags, func(flag string) string {
		return strings.TrimSpace(flag)
	})
	flags = slicekit.Map(flags, func(flag string) string {
		for strings.HasPrefix("-", flag) {
			flag = strings.TrimPrefix("-", flag)
		}
		return flag
	})
	return flags
}

type structMeta struct {
	New   func() any
	Flags []structFlag
	Args  []structArg
}

type structFlag struct {
	StructField reflect.StructField

	Default    string
	HasDefault bool
	DefVal     reflect.Value

	Names []string
	Desc  string

	Required bool
	Enum     []reflect.Value
}

func (sf structFlag) Setter(Struct reflect.Value, value flagValue) (rErr error) {
	name := strings.Join(sf.Names, "/")
	defer errorkit.Recover(&rErr)
	field := Struct.FieldByIndex(sf.StructField.Index)
	if !value.IsSet && sf.HasDefault { // use default value
		field.Set(sf.DefVal)
		return nil
	}
	if !value.IsSet {
		if sf.Required { // raise error for the missing but expected flag
			return ErrFlagMissing.F("%s flag is required", name)
		}
		return nil // ignore the flag, no value to be dependency injected
	}
	rval, err := convkit.ParseReflect(field.Type(), value.Raw)
	if err != nil {
		return ErrFlagParseIssue.F("%s (%s) encountered a parsing error with the value of: %q", name, field.Type().String(), value.Raw)
	}
	if err := checkEnum(sf.Enum, rval, name); err != nil {
		return err
	}
	field.Set(rval)
	return nil
}

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

func (sf structFlag) mapToFlagSet(fs *flag.FlagSet, Struct reflect.Value) func() error {
	var v = flagValue{Type: sf.StructField.Type}
	for _, n := range sf.Names {
		fs.Var(&v, n, sf.Default)
	}
	return func() error { return sf.Setter(Struct, v) }
	// switch sf.StructField.Type.Kind() {
	// case reflect.Bool:
	// 	var v bool
	// 	for _, name := range sf.Names {
	// 		fs.BoolVar(&v, name, sf.DefVal.Bool(), "")
	// 	}
	// 	return func() (rErr error) {
	// 		var raw string = "false"
	// 		if v {
	// 			raw = "true"
	// 		}
	// 		return sf.Setter(Struct, raw)
	// 	}
	// default:
	// 	var v string
	// 	for _, name := range sf.Names {
	// 		fs.StringVar(&v, name, sf.Default, "")
	// 	}
	// 	return func() error { return sf.Setter(Struct, v) }
	// }
}

type flagValue struct {
	Raw   string
	IsSet bool
	Type  reflect.Type
}

func (v *flagValue) String() string { return v.Raw }

func (v *flagValue) IsBoolFlag() bool { return v.Type.Kind() == reflect.Bool }

func (v *flagValue) Set(raw string) error {
	v.Raw = raw
	v.IsSet = true
	return nil
}

type structArg struct {
	StructField reflect.StructField

	Index int
	Name  string

	Default    string
	HasDefault bool
	DefVal     reflect.Value

	Desc     string
	Required bool
	Enum     []reflect.Value
}

func (sa structArg) Setter(Struct reflect.Value, raw string, ok bool) (rErr error) {
	if !ok && sa.Required {
		return ErrArgParseIssue.F("%s argument is not provided", sa.Name)
	}
	defer errorkit.Recover(&rErr)
	field := Struct.FieldByIndex(sa.StructField.Index)
	if !ok {
		if sa.HasDefault {
			field.Set(sa.DefVal)
		}
		return nil
	}
	rval, err := convkit.ParseReflect(field.Type(), raw)
	if err != nil {
		return ErrArgParseIssue.F("argument at index %d is not a %s type, and encountered a parsing error on %q: %s", sa.Index, field.Type().String(), raw, err.Error())
	}
	if err := checkEnum(sa.Enum, rval, sa.Name); err != nil {
		return err
	}
	field.Set(rval)
	return nil
}

func checkEnum(enums []reflect.Value, val reflect.Value, name string) error {
	if len(enums) == 0 {
		return nil
	}
	_, ok := slicekit.Find(enums, func(e reflect.Value) bool {
		return reflectkit.Equal(e, val)
	})
	if ok {
		return nil
	}

	var acceptedValues []string
	for _, val := range enums {
		fval, err := convkit.Format[any](val.Interface())
		if err != nil {
			return err
		}
		acceptedValues = append(acceptedValues, fval)
	}

	acceptedValuesFormatted := strings.Join(slicekit.Map(acceptedValues, func(v string) string { return " - " + v }), lineSeparator)

	return ErrFlagInvalid.F("%s got the value of %s which is not part of the acceptable values\n\naccepted values:\n%s", name, val.Interface(), acceptedValuesFormatted)
}
