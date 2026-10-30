package jsontoken

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"unicode/utf8"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
)

const ErrMalformed errorkitlite.Error = "[ErrMalformed] malformed JSON"

// LexingError reports malformed JSON input.
// It always matches ErrMalformed with errors.Is.
type LexingError struct {
	Message string
	Path    Path
	// Err is the underlying cause, typically a *jsontext.SyntacticError.
	Err error
}

func (err LexingError) Error() string {
	var (
		format string = "JSON lexing error"
		a      []any
		msg    = err.Message
	)
	if len(msg) == 0 && err.Err != nil {
		msg = err.Err.Error()
	}
	if 0 < len(msg) {
		format += ": %s"
		a = append(a, msg)
	}
	if 0 < len(err.Path) {
		format += "\n%s"
		a = append(a, err.Path.String())
	}
	return fmt.Sprintf(format, a...)
}

func (err LexingError) Unwrap() error { return err.Err }

func (err LexingError) Is(target error) bool { return target == ErrMalformed }

// Scanner is a streaming JSON lexer built on top of encoding/json/jsontext.
// It reads exactly one JSON value from its Input,
// and feeds the raw bytes of the values matching its Selectors to them as they are being read.
type Scanner struct {
	// Selectors allows granual control on what should be kept during scanning of a JSON input stream.
	// When no Selectors is set, the default is to keep everything.
	Selectors []Selector
	// BufferSize is the size of the reading buffer for each group of Selectors who match the same Selector#Path.
	//
	// default: 16 Mb
	BufferSize iokit.ByteSize
}

func (s *Scanner) On(path Path, on func(io.Reader) error) {
	s.Selectors = append(s.Selectors, Selector{
		Path: path,
		On:   on,
	})
}

type Selector struct {
	Path Path
	Func func(data []byte) error
	On   func(src io.Reader) error
}

var _ Input = (*bufio.Reader)(nil)

type Input interface {
	ReadRune() (r rune, size int, err error)
	UnreadRune() error
}

var _ Output = (*bytes.Buffer)(nil)

type Output interface {
	io.Writer
	WriteTo(w io.Writer) (n int64, err error)
	WriteRune(r rune) (n int, err error)
	WriteByte(c byte) error
	Bytes() []byte
}

// Scan reads a single JSON value from the input.
//
// The raw bytes of the value, including its insignificant whitespaces, are passed to the matching Selectors.
// Scanning stops right after the JSON value,
// and the rest of the input is left unread for the caller.
// An empty input is not considered an error.
func (s *Scanner) Scan(in Input) (rErr error) {
	if in == nil {
		return nil
	}
	src := newSource(in)
	st := &scanState{Scanner: s}
	st.dec = newDecoder(&flushingReader{src: src, st: st})
	defer func() { rErr = st.closeFrames(rErr) }()
	if err := st.scan(); err != nil {
		_ = src.unread(0)
		return err
	}
	if len(st.frames) == 0 { // empty input
		return src.unread(0)
	}
	return src.unread(len(st.dec.UnreadBuffer()))
}

func newDecoder(r io.Reader) *jsontext.Decoder {
	// These options make the decoder accept the same inputs as json.Valid.
	return jsontext.NewDecoder(r,
		jsontext.AllowDuplicateNames(true),
		jsontext.AllowInvalidUTF8(true))
}

type scanState struct {
	*Scanner
	dec *jsontext.Decoder
	// frames is the stack of the currently open JSON containers.
	// The first frame is the root scope that encloses the whole input,
	// including the whitespaces before the JSON value.
	frames []frame
	// err is the first failure of handing over data to the Selector.On readers.
	err error
	// scratch, index and the frame's name back the path of the value being read,
	// unless the value's frame needs to keep its path while reading its nested values.
	scratch Path
	index   int
	nameBuf []byte
	names   map[string]string
}

type frame struct {
	path  Path
	scope scope
	// selectors are the Selectors that might match a value nested within this frame.
	selectors []Selector
	// name is the last object member name read in this frame.
	name string
}

func (st *scanState) scan() error {
	for {
		kind := st.dec.PeekKind()
		if kind == jsontext.KindInvalid {
			_, err := st.dec.ReadToken()
			if errors.Is(err, io.EOF) && len(st.frames) == 0 {
				return nil // empty input
			}
			return st.wrapErr(err)
		}
		if len(st.frames) == 0 {
			st.frames = append(st.frames, frame{
				scope:     st.open(scope{}, st.Selectors, nil),
				selectors: within(st.Selectors, nil),
			})
		}
		top := &st.frames[len(st.frames)-1]
		if 0 < len(top.scope.sinks) {
			// PeekKind already buffered the whitespaces and the ',' or ':' separator before the next token,
			// and these belong to the enclosing scope.
			if err := top.scope.write(separatorPrefix(st.dec.UnreadBuffer())); err != nil {
				return err
			}
		}
		var err error
		switch kind {
		case jsontext.KindBeginArray, jsontext.KindBeginObject:
			err = st.scanBegin(top, kind)
		case jsontext.KindEndArray, jsontext.KindEndObject:
			err = st.scanEnd(top, kind)
		default:
			err = st.scanScalar(top, kind)
		}
		if err != nil {
			return err
		}
		if st.dec.StackDepth() == 0 {
			return nil
		}
	}
}

func (st *scanState) scanBegin(top *frame, kind jsontext.Kind) error {
	path, _ := st.appendChildPath(st.scratch[:0], top, toKind(kind), false)
	st.scratch = path
	sc := st.open(top.scope, top.selectors, path)
	if selectors := within(top.selectors, path); 0 < len(selectors) {
		path, _ = st.appendChildPath(make(Path, 0, len(path)), top, toKind(kind), true)
		st.frames = append(st.frames, frame{path: path, scope: sc, selectors: selectors}) // top is invalidated
		if _, err := st.dec.ReadToken(); err != nil {
			return st.wrapErr(err)
		}
		return sc.write(delimiter(kind))
	}
	// No Selector can match anything nested in this value, so it can be read as a whole.
	// The frame is only kept for flushing its scope and reporting errors, so it can use the scratch path.
	st.frames = append(st.frames, frame{path: path, scope: sc})
	if err := st.consume(sc); err != nil {
		return st.wrapErr(err)
	}
	st.frames = st.frames[:len(st.frames)-1]
	return st.close(sc, nil)
}

func (st *scanState) scanEnd(top *frame, kind jsontext.Kind) error {
	if _, err := st.dec.ReadToken(); err != nil {
		return st.wrapErr(err)
	}
	sc := top.scope
	st.frames = st.frames[:len(st.frames)-1]
	return st.close(sc, sc.write(delimiter(kind)))
}

func (st *scanState) scanScalar(top *frame, kind jsontext.Kind) error {
	if len(top.selectors) == 0 {
		val, err := st.dec.ReadValue()
		if err != nil {
			return st.wrapErr(err)
		}
		return top.scope.write(val)
	}
	path, isName := st.appendChildPath(st.scratch[:0], top, toKind(kind), false)
	st.scratch = path
	val, err := st.dec.ReadValue()
	if err != nil {
		return st.wrapErr(err)
	}
	if isName {
		// the error can be ignored, as the decoder already validated the string
		st.nameBuf, _ = jsontext.AppendUnquote(st.nameBuf[:0], val)
		top.name = st.intern(st.nameBuf)
	}
	sc := st.open(top.scope, top.selectors, path)
	return st.close(sc, sc.write(val))
}

// consume reads the next JSON value as a whole, and writes it to the scope.
func (st *scanState) consume(sc scope) error {
	switch {
	case len(sc.sinks) == 0:
		return st.dec.SkipValue()
	case len(sc.pipes) == 0:
		val, err := st.dec.ReadValue()
		if err != nil {
			return err
		}
		return sc.write(val)
	}
	// A Selector.On reader is consuming the value as it is being read,
	// so the value is passed on token by token instead of buffering it whole.
	depth := st.dec.StackDepth()
	for first := true; ; first = false {
		kind := st.dec.PeekKind()
		if !first { // the separator before the value is already written by the caller
			if err := sc.write(separatorPrefix(st.dec.UnreadBuffer())); err != nil {
				return err
			}
		}
		switch kind {
		case jsontext.KindBeginArray, jsontext.KindBeginObject, jsontext.KindEndArray, jsontext.KindEndObject:
			if _, err := st.dec.ReadToken(); err != nil {
				return err
			}
			if err := sc.write(delimiter(kind)); err != nil {
				return err
			}
		default:
			val, err := st.dec.ReadValue()
			if err != nil {
				return err
			}
			if err := sc.write(val); err != nil {
				return err
			}
		}
		if st.dec.StackDepth() == depth {
			return nil
		}
	}
}

// appendChildPath appends the path of the next value in the top frame to dst,
// and reports whether that value is an object member name.
// Unless stable is set, the path refers to the scanning state,
// and it is only valid until the next value is read.
func (st *scanState) appendChildPath(dst Path, top *frame, k Kind, stable bool) (Path, bool) {
	dst = append(dst, top.path...)
	switch ck, n := st.dec.StackIndex(st.dec.StackDepth()); ck {
	case jsontext.KindBeginArray:
		index := &st.index
		if stable {
			index = new(int)
		}
		*index = int(n)
		return append(dst, KindElement{Index: index}, k), false
	case jsontext.KindBeginObject:
		if n%2 == 0 {
			return append(dst, KindName, k), true
		}
		name := &top.name
		if stable {
			name = new(string)
			*name = top.name
		}
		return append(dst, KindValue{Name: name}, k), false
	default:
		return append(dst, k), false
	}
}

// intern deduplicates the object member names, as the same names tend to repeat throughout the input.
func (st *scanState) intern(name []byte) string {
	if s, ok := st.names[string(name)]; ok {
		return s
	}
	const maxInternedNames = 1024
	s := string(name)
	if st.names == nil {
		st.names = make(map[string]string)
	}
	if len(st.names) < maxInternedNames {
		st.names[s] = s
	}
	return s
}

// within returns the selectors that might match a value nested within the value of the given path.
func within(selectors []Selector, path Path) []Selector {
	for i, selector := range selectors {
		if mightMatchWithin(selector.Path, path) {
			continue
		}
		out := append([]Selector(nil), selectors[:i]...)
		for _, selector := range selectors[i+1:] {
			if mightMatchWithin(selector.Path, path) {
				out = append(out, selector)
			}
		}
		return out
	}
	return selectors
}

// mightMatchWithin reports whether the selector path could match any value nested within the value of the path.
// The paths of the nested values are longer than the path, and they start with it.
func mightMatchWithin(selector, path Path) bool {
	if len(selector) <= len(path) {
		return false
	}
	for i := range path {
		if !selector[i].Match(path[i]) {
			return false
		}
	}
	return true
}

// flush hands over the data processed so far to the Selector.On readers.
func (st *scanState) flush() error {
	if st.err != nil {
		return st.err
	}
	for i := range st.frames {
		if p := st.frames[i].scope.pipe; p != nil {
			if err := p.flush(); err != nil {
				st.err = err
				return err
			}
		}
	}
	return nil
}

// flushingReader flushes the processed data to the Selector.On readers before reading from the Input,
// so they are not kept waiting while the Input blocks for more data.
type flushingReader struct {
	src io.Reader
	st  *scanState
}

func (r *flushingReader) Read(p []byte) (int, error) {
	if err := r.st.flush(); err != nil {
		return 0, err
	}
	return r.src.Read(p)
}

func (st *scanState) wrapErr(err error) error {
	if st.err != nil { // the decoder only failed because of it
		return st.err
	}
	if serr, ok := errorkitlite.As[*jsontext.SyntacticError](err); ok {
		var path Path
		if 0 < len(st.frames) {
			path = ownPath(st.frames[len(st.frames)-1].path)
		}
		return LexingError{Path: path, Err: serr}
	}
	return err
}

// ownPath copies the path, including the values its kinds refer to,
// so it no longer depends on the scanning state.
func ownPath(p Path) Path {
	if len(p) == 0 {
		return nil
	}
	out := make(Path, len(p))
	for i, k := range p {
		switch k := k.(type) {
		case KindElement:
			if k.Index != nil {
				index := *k.Index
				k.Index = &index
			}
			out[i] = k
		case KindValue:
			if k.Name != nil {
				name := *k.Name
				k.Name = &name
			}
			out[i] = k
		default:
			out[i] = k
		}
	}
	return out
}

// closeFrames closes all the open frames from the innermost to the outermost.
func (st *scanState) closeFrames(err error) error {
	for 0 < len(st.frames) {
		last := len(st.frames) - 1
		sc := st.frames[last].scope
		st.frames = st.frames[:last]
		err = st.close(sc, err)
	}
	return err
}

func separatorPrefix(data []byte) []byte {
	for i, c := range data {
		switch c {
		case ' ', '\t', '\r', '\n', ',', ':':
		default:
			return data[:i]
		}
	}
	return data
}

var delimiters = []byte("[]{}")

func delimiter(k jsontext.Kind) []byte {
	i := bytes.IndexByte(delimiters, byte(k))
	return delimiters[i : i+1]
}

func toKind(k jsontext.Kind) Kind {
	switch k {
	case jsontext.KindBeginArray, jsontext.KindEndArray:
		return KindArray
	case jsontext.KindBeginObject, jsontext.KindEndObject:
		return KindObject
	case jsontext.KindString:
		return KindString
	case jsontext.KindNumber:
		return KindNumber
	case jsontext.KindTrue, jsontext.KindFalse:
		return KindBoolean
	case jsontext.KindNull:
		return KindNull
	default:
		return nil
	}
}

func (s *Scanner) getBufferSize() iokit.ByteSize {
	const DefaultBufferSize = 16 * iokit.Megabyte
	return cmp.Or(s.BufferSize, DefaultBufferSize)
}

// scope is the output of a JSON value,
// that feeds every byte written to it to the Selectors matching the value's Path,
// and to the Selectors of its enclosing scopes.
type scope struct {
	// sinks are the buffers of every Selector that receives the data written to the scope,
	// including the ones of the enclosing scopes.
	sinks []*bytes.Buffer
	// pipes are the Selector.On pipes among the sinks.
	pipes []*pipe

	fns  []func(data []byte) error
	buf  *bytes.Buffer
	pipe *pipe
	g    *synckit.Group
}

func (s *Scanner) open(parent scope, selectors []Selector, path Path) scope {
	var (
		sc  = scope{sinks: parent.sinks, pipes: parent.pipes}
		ons []func(src io.Reader) error
	)
	for _, selector := range selectors {
		if !selector.Path.Match(path) {
			continue
		}
		if selector.Func != nil {
			sc.fns = append(sc.fns, selector.Func)
		}
		if selector.On != nil {
			ons = append(ons, selector.On)
		}
	}
	// the parent's slices are clipped, so the scope's sinks are never appended to the parent's backing array.
	if 0 < len(sc.fns) {
		sc.buf = &bytes.Buffer{}
		sc.sinks = append(slices.Clip(sc.sinks), sc.buf)
	}
	if 0 < len(ons) {
		pr, pw := io.Pipe()
		sc.pipe, sc.g = &pipe{w: pw}, &synckit.Group{}
		sc.sinks = append(slices.Clip(sc.sinks), &sc.pipe.pending)
		sc.pipes = append(slices.Clip(sc.pipes), sc.pipe)
		if len(ons) == 1 {
			sc.g.Go(nil, func(ctx context.Context) error {
				defer pr.Close()
				return ons[0](pr)
			})
		} else {
			for i, reader := range iokit.LockstepReaders(pr, len(ons), s.getBufferSize()) {
				sc.g.Go(nil, func(ctx context.Context) error {
					defer reader.Close()
					return ons[i](reader)
				})
			}
		}
	}
	return sc
}

func (sc scope) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	for _, sink := range sc.sinks {
		sink.Write(data)
	}
	for _, p := range sc.pipes {
		if pipeBatchSize <= p.pending.Len() {
			if err := p.flush(); err != nil {
				return err
			}
		}
	}
	return nil
}

// pipeBatchSize is the amount of data that is handed over at once to the Selector.On readers.
const pipeBatchSize = 32 * 1024

// pipe hands over the data to the Selector.On readers in batches,
// as each hand over is a context switch between goroutines.
// The pending data is flushed when the batch is full, when its value is complete,
// and before the scanning would wait for more input.
type pipe struct {
	w       *io.PipeWriter
	pending bytes.Buffer
}

func (p *pipe) flush() error {
	if p.pending.Len() == 0 {
		return nil
	}
	_, err := p.w.Write(p.pending.Bytes())
	p.pending.Reset()
	return err
}

// abortError is how the readers of the Selector.On functions learn that scanning failed.
// It is a pointer, so the error coming back from them can be recognised and dropped,
// as it would only duplicate the original error.
type abortError struct{ err error }

func (e *abortError) Error() string { return e.err.Error() }
func (e *abortError) Unwrap() error { return e.err }

// close finishes the scope, which must be already removed from the frames.
// The Selector.Func functions are only called when the scope is complete without an error.
func (st *scanState) close(sc scope, err error) error {
	if sc.pipe == nil && len(sc.fns) == 0 {
		return err
	}
	if err == nil {
		// The enclosing scopes' Selector.On readers receive the data
		// before the Selectors of this scope learn that the value is complete.
		err = st.flush()
	}
	if sc.pipe != nil {
		err = sc.closePipe(err)
	}
	if err != nil {
		return err
	}
	for _, fn := range sc.fns {
		if err := fn(sc.buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (sc scope) closePipe(err error) error {
	if err == nil {
		err = sc.pipe.flush()
	}
	var abort *abortError
	if err != nil {
		abort = &abortError{err: err}
		_ = sc.pipe.w.CloseWithError(abort)
	} else {
		_ = sc.pipe.w.Close()
	}
	if gErr := sc.g.Wait(); gErr != nil && (abort == nil || !errors.Is(gErr, abort)) {
		if errors.Is(err, io.ErrClosedPipe) {
			// a Selector.On returned early, and its error is the root cause of the failed write.
			err = gErr
		} else {
			err = errorkitlite.Merge(err, gErr)
		}
	}
	return err
}

// source adapts an Input to an io.Reader for the jsontext.Decoder,
// and allows giving back the decoder's unused look-ahead to the Input.
type source interface {
	io.Reader
	// unread returns the last n bytes from the last Read call to the underlying Input.
	unread(n int) error
}

func newSource(in Input) source {
	if in, ok := in.(bufferedInput); ok {
		return &bufferedSource{in: in}
	}
	if in, ok := in.(io.ReadSeeker); ok {
		if _, err := in.Seek(0, io.SeekCurrent); err == nil {
			return seekSource{in: in}
		}
	}
	if in, ok := in.(io.ByteScanner); ok {
		return byteSource{in: in}
	}
	return &runeSource{in: in}
}

func errUnread(n int) error {
	return fmt.Errorf("jsontoken: unable to give back %d bytes of look-ahead to the Input", n)
}

// bufferedInput is implemented by *bufio.Reader.
type bufferedInput interface {
	Peek(n int) ([]byte, error)
	Discard(n int) (discarded int, err error)
	Buffered() int
}

// bufferedSource lends the already buffered bytes of the Input to the decoder,
// and only discards them from the Input once the decoder has consumed them.
type bufferedSource struct {
	in   bufferedInput
	lent int
}

func (s *bufferedSource) Read(p []byte) (int, error) {
	if err := s.commit(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if s.in.Buffered() == 0 {
		if _, err := s.in.Peek(1); err != nil {
			return 0, err
		}
	}
	data, err := s.in.Peek(min(s.in.Buffered(), len(p)))
	s.lent = copy(p, data)
	return s.lent, err
}

func (s *bufferedSource) commit() error {
	if s.lent == 0 {
		return nil
	}
	_, err := s.in.Discard(s.lent)
	s.lent = 0
	return err
}

func (s *bufferedSource) unread(n int) error {
	if s.lent < n {
		_ = s.commit()
		return errUnread(n)
	}
	s.lent -= n
	return s.commit()
}

type seekSource struct{ in io.ReadSeeker }

func (s seekSource) Read(p []byte) (int, error) { return s.in.Read(p) }

func (s seekSource) unread(n int) error {
	if n == 0 {
		return nil
	}
	_, err := s.in.Seek(-int64(n), io.SeekCurrent)
	return err
}

// byteSource reads one byte at a time, so at most a single byte needs to be given back.
type byteSource struct{ in io.ByteScanner }

func (s byteSource) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c, err := s.in.ReadByte()
	if err != nil {
		return 0, err
	}
	p[0] = c
	return 1, nil
}

func (s byteSource) unread(n int) error {
	switch n {
	case 0:
		return nil
	case 1:
		return s.in.UnreadByte()
	default:
		return errUnread(n)
	}
}

// runeSource reads one rune at a time, so at most a single rune needs to be given back.
type runeSource struct {
	in      Input
	buf     [utf8.UTFMax]byte
	size    int
	pending []byte
}

func (s *runeSource) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(s.pending) == 0 {
		r, _, err := s.in.ReadRune()
		if err != nil {
			return 0, err
		}
		s.size = utf8.EncodeRune(s.buf[:], r)
		s.pending = s.buf[:s.size]
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func (s *runeSource) unread(n int) error {
	switch returned := s.size - len(s.pending); {
	case n == 0 && len(s.pending) == 0:
		return nil
	case 0 < n && n == returned:
		s.size, s.pending = 0, nil
		return s.in.UnreadRune()
	default:
		return errUnread(n)
	}
}

/* KIND */

type Path []Kind

func (p Path) String() string   { return strings.Join(slicekit.Map(p, Kind.String), " -> ") }
func (p Path) With(k Kind) Path { return append(slicekit.Clone(p), k) }

// Contains check if the other path matches the
func (p Path) Contains(oth Path) bool {
	if len(p) == 0 {
		return true
	}
	if len(oth) < len(p) {
		return false
	}
	for i := range p {
		if !p[i].Match(oth[i]) {
			return false
		}
	}
	return true
}

func (p Path) Match(oth Path) bool {
	if p.Equal(oth) {
		return true
	}
	// len(p)-1 defines that if the p Path is an array element path,
	// then potentially an any value index selector
	// is accepted as a matching path.
	if len(p) < 2 || len(oth) != len(p)+1 {
		return false
	}
	if !p.Contains(oth) { //  smoke test that p match to oth
		return false
	}
	var last = p[len(p)-1]
	switch last.(type) { // last
	case KindElement, KindValue:
		// if the p's last elem is a Element / Value kind,
		// then since contains already matched, we consider it as a full match
		return true
	}
	if last.Match(KindName) { // Name is also accepted as a container selector pat to Object name string values.
		return true
	}
	return false
}

func (p Path) Equal(oth Path) bool {
	return len(p) == len(oth) && p.Contains(oth)
}

type Kind interface {
	Match(Kind) bool
	String() string
}

type strKind string

func (sk strKind) Match(oth Kind) bool {
	osk, ok := oth.(strKind)
	if !ok {
		return false
	}
	return sk == osk
}

func (sk strKind) String() string {
	return string(sk)
}

const (
	KindString  strKind = "string"
	KindNumber  strKind = "number"
	KindBoolean strKind = "boolean"
	KindNull    strKind = "null"
)

const KindArray strKind = "array"

type KindElement struct{ Index *int }

func (e KindElement) Match(oth Kind) bool {
	oe, ok := oth.(KindElement)
	if !ok {
		return false
	}
	if e.Index == nil || oe.Index == nil {
		return true
	}
	return *e.Index == *oe.Index
}

func (e KindElement) String() string {
	if e.Index == nil {
		return "[]"
	}
	return fmt.Sprintf("[%d]", *e.Index)
}

const (
	KindObject strKind = "object"
	KindName   strKind = "name"
)

// KindValue is the path element of an object member's value.
// Name is the decoded (unescaped) member name.
type KindValue struct{ Name *string }

func (v KindValue) Match(oth Kind) bool {
	other, ok := oth.(KindValue)
	if !ok {
		return false
	}
	if v.Name == nil || other.Name == nil {
		return true
	}
	return *v.Name == *other.Name
}

func (v KindValue) String() string {
	if v.Name == nil {
		return ".*"
	}
	if len(*v.Name) == 0 {
		return `[""]`
	}
	return "." + *v.Name
}

var _ = enum.Register[Kind](
	KindArray,
	KindObject,
	KindString,
	KindNumber,
	KindBoolean,
	KindNull,
	KindElement{},
	KindValue{},
)

func IterateArray(ctx context.Context, r io.Reader) iter.Seq2[json.RawMessage, error] {
	i := &ArrayIterator{Context: ctx, Input: r}
	return iterkit.Once2(func(yield func(json.RawMessage, error) bool) {
		defer i.Close()
		for i.Next() {
			if !yield(i.Value(), nil) {
				return
			}
		}
		if err := i.Err(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
		if err := i.Close(); err != nil {
			if !yield(nil, err) {
				return
			}
		}
	})
}

type ArrayIterator struct {
	Context context.Context
	Input   io.Reader

	dec *jsontext.Decoder

	index  int
	inList bool

	err  error
	done bool

	data []byte
}

func (c *ArrayIterator) Next() bool {
	if c.done {
		return false
	}
	if err := c.Err(); err != nil {
		return false
	}
	if c.dec == nil {
		c.dec = newDecoder(c.Input)
	}

	if !c.inList {
		tkn, err := c.dec.ReadToken()
		if err != nil {
			c.err = c.wrapErr(err, Path{KindArray})
			return false
		}
		if kind := tkn.Kind(); kind != jsontext.KindBeginArray {
			c.err = LexingError{
				Message: fmt.Sprintf(`unexpected json token, expected "[" but got %s`, kind),
				Path:    Path{KindArray},
			}
			return false
		}
		c.inList = true
		c.index = 0
	}

	if c.dec.PeekKind() == jsontext.KindEndArray {
		if _, err := c.dec.ReadToken(); err != nil {
			c.err = c.wrapErr(err, Path{KindArray})
			return false
		}
		c.done = true
		return false
	}

	raw, err := c.dec.ReadValue()
	if err != nil {
		index := c.index
		c.err = c.wrapErr(err, Path{KindArray, KindElement{Index: &index}})
		return false
	}
	c.index++
	c.data = raw.Clone()
	return true
}

func (c *ArrayIterator) wrapErr(err error, path Path) error {
	if serr, ok := errorkitlite.As[*jsontext.SyntacticError](err); ok {
		return LexingError{Path: path, Err: serr}
	}
	return err
}

func (c *ArrayIterator) Value() json.RawMessage { return c.data }

func (c *ArrayIterator) Decode(ptr any) error { return json.Unmarshal(c.data, ptr) }

func (c *ArrayIterator) Err() error {
	if c.Context == nil {
		return c.err
	}
	return errorkit.Merge(c.err, c.Context.Err())
}

func (c *ArrayIterator) Close() error {
	if c, ok := c.Input.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// TrimSpace removes the insignificant whitespaces from a JSON value.
// Whitespaces within JSON strings are kept intact.
func TrimSpace(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		switch data[i] {
		case ' ', '\t', '\n', '\r':
			i++
		case '"':
			end := stringEnd(data, i+1)
			out = append(out, data[i:end]...)
			i = end
		default:
			j := i + 1
			for j < len(data) && !isSpaceOrQuote(data[j]) {
				j++
			}
			out = append(out, data[i:j]...)
			i = j
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isSpaceOrQuote(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '"'
}

// stringEnd returns the index right after the closing quote of the JSON string that starts at i,
// or the length of the data when the string is unterminated.
func stringEnd(data []byte, i int) int {
	for {
		j := bytes.IndexByte(data[i:], '"')
		if j < 0 {
			return len(data)
		}
		i += j
		// the quote is escaped when it is preceded by an odd number of backslashes
		var backslashes int
		for k := i - 1; 0 <= k && data[k] == '\\'; k-- {
			backslashes++
		}
		i++
		if backslashes%2 == 0 {
			return i
		}
	}
}
