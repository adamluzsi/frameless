package jsontoken

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

const ErrMalformed errorkit.Error = "[ErrMalformed] malformed JSON"

type LexingError struct {
	Message string
	Path    Path
}

func (err LexingError) Error() string {
	var (
		format string = "JSON lexing error"
		a      []any
	)
	if 0 < len(err.Message) {
		format += ": %s"
		a = append(a, err.Message)
	}
	if 0 < len(err.Path) {
		format += "\n%s"
		a = append(a, err.Path.String())
	}
	return fmt.Sprintf(format, a...)
}

type token string

func (tkn token) Bytes() []byte {
	return []byte(tkn)
}

const (
	nullToken  token = "null"
	trueToken  token = "true"
	falseToken token = "false"

	quoteToken = '"'

	arrayOpenToken  = '['
	arrayCloseToken = ']'
	valueSepToken   = ','

	objectOpenToken  = '{'
	objectCloseToken = '}'
	nameSepToken     = ':'
)

var (
	nullTokenUTF8  = []rune(nullToken)
	trueTokenUTF8  = []rune(trueToken)
	falseTokenUTF8 = []rune(falseToken)
)

// Scanner is a streaming lexer, that allows
type Scanner struct {
	// Selectors allows granual control on what should be kept during scanning of a JSON input stream.
	// When no Selectors is set, the default is to keep everything.
	Selectors []Selector
}

func (s *Scanner) isPathMatch(path Path) bool {
	if len(s.Selectors) == 0 {
		return true
	}
	for _, f := range s.Selectors {
		if f.Path.Match(path) {
			return true
		}
	}
	return false
}

type Selector struct {
	Path Path
	Func func(data json.RawMessage) error
}

var _ Input = (*bufio.Reader)(nil)

type Input interface {
	iokit.RuneReader
	iokit.RuneUnreader
	iokit.ByteReader
}

var _ Output = (*bytes.Buffer)(nil)

type Output interface {
	io.Writer
	WriteTo(w io.Writer) (n int64, err error)
	WriteRune(r rune) (n int, err error)
	WriteByte(c byte) error
	Bytes() []byte
}

type noDiscard interface {
	NoDiscard()
}

func (s *Scanner) Scan(in Input) error {
	var path Path
	var out bytes.Buffer
	err := s.with(&out, path, func(out Output) error {
		return s.scan(in, out, path)
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return s.asLexingError(err, path)
	}
	if err := s.yield(path, out.Bytes()); err != nil {
		return err
	}
	return nil
}

func (s *Scanner) yield(path Path, data json.RawMessage) error {
	var o sync.Once
	for _, selector := range s.Selectors {
		if selector.Path.Equal(path) && selector.Func != nil {
			var err error
			o.Do(func() {
				if !json.Valid(data) {
					err = LexingError{
						Message: "invalid JSON format",
						Path:    path,
					}
				}
			})
			if err != nil {
				return err
			}
			if err := selector.Func(data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Scanner) asLexingError(err error, path Path) error {
	if err == nil {
		return nil
	}
	if _, ok := errorkit.As[LexingError](err); ok {
		return err
	}
	return errorkit.Merge(err, LexingError{Path: path})
}

func (s *Scanner) scan(in Input, out Output, path Path) error {
	if in == nil {
		return nil
	}
	if err := trimSpace(in, out); err != nil {
		return err
	}
	char, _, err := iokit.PeekRune(in)
	if err != nil {
		return err
	}
	switch kind := s.tokenStartKind(char); kind {
	case KindNull:
		return s.scanNull(in, out, path)
	case KindBoolean:
		return s.scanBoolean(in, out, path)
	case KindString:
		return s.scanString(in, out, path)
	case KindNumber:
		return s.scanNumber(in, out, path)
	case KindArray:
		return s.scanArray(in, out, path)
	case KindObject:
		return s.scanObject(in, out, path)
	default:
		var msg string = fmt.Sprintf("not-implemented, unhandled character: %q", string(char))
		if kind != nil {
			msg += fmt.Sprintf("\nkind=%s", kind.String())
		}
		return LexingError{
			Message: msg,
			Path:    path,
		}
	}
}

func (s *Scanner) with(out Output, path Path, blk func(out Output) error) error {
	var raw Output = &bytes.Buffer{}
	if !s.isPathMatch(path) {
		if _, ok := out.(noDiscard); !ok {
			raw = discard
		}
	}
	returnErr := blk(raw)
	if returnErr != nil && !errors.Is(returnErr, io.EOF) { // EOF is a good type of error, signaling the end of the input stream
		return returnErr
	}
	if err := s.yield(path, raw.Bytes()); err != nil {
		return err
	}
	if _, err := raw.WriteTo(out); err != nil {
		return err
	}
	return returnErr
}

// copyTo will not exhaust the input buffer but retains its content.
func copyTo(in, out Output) error {
	for _, c := range in.Bytes() {
		if err := out.WriteByte(c); err != nil {
			return err
		}
	}
	return nil
}

func trimSpace(in Input, out Output) error {
	for {
		char, _, err := in.ReadRune()
		if err != nil {
			return err
		}
		if _, ok := whitespaceChars[char]; !ok {
			return in.UnreadRune()
		}
		out.WriteRune(char)
	}
}

func (s *Scanner) scanNumber(in Input, out Output, path Path) error {
	path = path.With(KindNumber)
	return s.with(out, path, func(out Output) error {
	scan:
		for {
			digit, _, err := in.ReadRune()
			if err != nil {
				return err
			}
			if _, ok := numberChars[digit]; !ok {
				// no more number chars, we are ready,
				// the last read should be reverted.
				if err := in.UnreadRune(); err != nil {
					return err
				}
				break scan
			}
			if _, err := out.WriteRune(digit); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Scanner) scanNull(in Input, out Output, path Path) error {
	return s.scanToken(in, out, path.With(KindNull), nullTokenUTF8)
}

func (s *Scanner) scanToken(in Input, out Output, path Path, token []rune) error {
	return s.with(out, path, func(out Output) error {
		for i := 0; i < len(token); i++ {
			char, _, err := in.ReadRune()
			if err != nil {
				return LexingError{Message: "error while reading from input: " + err.Error(), Path: path}
			}
			if char != token[i] {
				return LexingError{Message: fmt.Sprintf(`error parsing %q token: expected "%q" but got "%c"`, string(token), char, token[i]), Path: path}
			}
			if _, err := out.WriteRune(char); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Scanner) scanBoolean(in Input, out Output, path Path) error {
	path = path.With(KindBoolean)
	if err := trimSpace(in, out); err != nil {
		return err
	}
	char, _, err := iokit.PeekRune(in)
	if err != nil {
		return err
	}
	switch char {
	case 't':
		return s.scanToken(in, out, path, trueTokenUTF8)
	case 'f':
		return s.scanToken(in, out, path, falseTokenUTF8)
	default:
		return LexingError{Message: fmt.Sprintf("unexpected boolean first character: %c", char), Path: path}
	}
}

func (s *Scanner) scanArray(in Input, out Output, path Path) error {
	path = path.With(KindArray)
	return s.with(out, path, func(out Output) error {
		if err := trimSpace(in, out); err != nil {
			return err
		}

		firstChar, _, err := iokit.MoveRune(in, out)
		if err != nil {
			return err
		}
		if firstChar != arrayOpenToken {
			return LexingError{Message: fmt.Sprintf(`unexpected array open token, expected "[" but got %c`, firstChar), Path: path}
		}

		secondChar, _, err := iokit.PeekRune(in)
		if err != nil {
			return err
		}
		if secondChar == arrayCloseToken { // empty array
			_, _, err := iokit.MoveRune(in, out)
			return err
		}

		{ // check for empty array
			if err := trimSpace(in, out); err != nil {
				return err
			}

			nextChar, _, err := iokit.PeekRune(in)
			if err != nil {
				return err
			}
			if nextChar == arrayCloseToken {
				if _, _, err := iokit.MoveRune(in, out); err != nil {
					return err
				}
				return nil
			}
		}

	scanValues:
		for i := 0; ; i++ {
			if err := trimSpace(in, out); err != nil {
				return err
			}

			// scan array value
			if err := s.scan(in, out, path.With(KindElement{Index: &i})); err != nil {
				return err
			}

			if err := trimSpace(in, out); err != nil {
				return err
			}

			// scan sep/close
			next, _, err := iokit.MoveRune(in, out)
			if err != nil {
				return err
			}
			switch next {
			case valueSepToken: // has more
				continue scanValues
			case arrayCloseToken:
				break scanValues
			default:
				return LexingError{Message: fmt.Sprintf("unexpected array token: %c", next), Path: path}
			}
		}
		return nil
	})
}

func (s *Scanner) scanObject(in Input, out Output, path Path) error {
	path = path.With(KindObject)
	return s.with(out, path, func(out Output) error {
		if err := trimSpace(in, out); err != nil {
			return err
		}

		firstChar, _, err := iokit.MoveRune(in, out)
		if err != nil {
			return err
		}

		if firstChar != objectOpenToken { // '{'
			return LexingError{Message: fmt.Sprintf(`unexpected object open token, expected "{" but got %c`, firstChar), Path: path}
		}

		if err := trimSpace(in, out); err != nil {
			return err
		}

		secondChar, _, err := iokit.PeekRune(in)
		if err != nil {
			return err
		}
		if secondChar == objectCloseToken { // empty object
			_, _, err := iokit.MoveRune(in, out) // write '}'
			return err
		}

	scan:
		for {
			if err := trimSpace(in, out); err != nil {
				return err
			}

			{ /* key-value pair */

				/* SCAN STRING KEY */

				// we need to make sure that the object name is retrieved
				// and not discarded from the output writing.
				var name objectKeyBuffer
				if err := s.scanString(in, &name, path.With(KindName)); err != nil {
					return fmt.Errorf("(object key) %w", err)
				}

				if err := copyTo(&name, out); err != nil {
					return err
				}

				if err := trimSpace(in, out); err != nil {
					return err
				}

				/* SEPERATOR */
				sep, _, err := iokit.MoveRune(in, out)
				if err != nil {
					return err
				}
				if sep != nameSepToken {
					return LexingError{Message: fmt.Sprintf(`unexpected object key-value separator, expected ":" but got "%c"`, sep), Path: path}
				}
				if err := trimSpace(in, out); err != nil {
					return err
				}

				/* SCAN OBJECT VALUE */
				var valueName string = name.String()
				if rawName := name.Bytes(); 2 <= len(rawName) {
					valueName = string(rawName[1 : len(rawName)-1]) // drop quote tokens
				}

				if err := s.scan(in, out, path.With(KindValue{Name: &valueName})); err != nil {
					return err
				}
				if err := trimSpace(in, out); err != nil {
					return err
				}
			}

			next, _, err := iokit.MoveRune(in, out)
			if err != nil {
				return err
			}
			switch next {
			case objectCloseToken:
				return nil
			case valueSepToken:
				continue scan
			default:
				return LexingError{
					Message: fmt.Sprintf(`unexpected character in object, expected either "," or "}", but got "%c"`, next),
					Path:    path,
				}
			}
		}
	})
}

func (s *Scanner) scanString(in Input, out Output, path Path) error {
	path = path.With(KindString)
	return s.with(out, path, func(out Output) error {
		if err := trimSpace(in, out); err != nil {
			return err
		}
		var str bytes.Buffer
		first, _, err := iokit.MoveRune(in, &str)
		if err != nil {
			return err
		}
		if first != quoteToken {
			return LexingError{
				Message: fmt.Sprintf(`unexpected string starting token, expected quote but got "%c"`, first),
				Path:    path,
			}
		}
	scan:
		for {
			b, err := iokit.MoveByte(in, &str)
			if err != nil {
				return err
			}
			if b == byte(quoteToken) {
				// it is only enough to check if the string is fully found when we see a potential closing quote character.
				// this way, we don't need to check the validity on each utf8 character.
				if json.Valid(str.Bytes()) {
					break scan
				}
			}
		}
		if _, err := str.WriteTo(out); err != nil {
			return err
		}
		return nil
	})
}

func (s *Scanner) tokenStartKind(char rune) Kind {
	if _, ok := numberChars[char]; ok {
		return KindNumber
	}
	if _, ok := whitespaceChars[char]; ok {
		return nil
	}
	switch char {
	case '[':
		return KindArray
	case '{':
		return KindObject
	case '"':
		return KindString
	case 'n':
		return KindNull
	case 't', 'f':
		return KindBoolean
	}
	return nil
}

var whitespaceChars = map[rune]struct{}{
	' ':  {},
	'\n': {},
	'\r': {},
	'\t': {},
}

var numberChars = map[rune]struct{}{
	'0': {},
	'1': {},
	'2': {},
	'3': {},
	'4': {},
	'5': {},
	'6': {},
	'7': {},
	'8': {},
	'9': {},
	'.': {},
	'-': {},
	'+': {},
	'e': {},
	'E': {},
}

/* KIND */

type Path []Kind

func (p Path) String() string   { return strings.Join(slicekit.Map(p, Kind.String), " -> ") }
func (p Path) With(k Kind) Path { return append(slicekit.Clone(p), k) }

// Match check if the other path matches the
func (p Path) Match(oth Path) bool {
	if len(p) == 0 {
		return true
	}
	if len(oth) < len(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		if !p[i].Equal(oth[i]) {
			return false
		}
	}
	return true
}

func (p Path) Equal(oth Path) bool {
	if len(p) == len(oth) {
		return p.Match(oth)
	}
	if 2 <= len(p) && len(oth) == len(p)+1 {
		if !p.Match(oth) { //  smoke test that p match to oth
			return false
		}
		pLastIndex := len(p) - 1
		pLastKind := p[pLastIndex]
		if pLastKind.Equal(KindElement{}) {
			return true
		}
		if pLastKind.Equal(KindName) {
			return true
		}
		if (KindValue{}).Equal(pLastKind) {
			return true //  pLastKind.Equal(oth[pLastIndex])
		}
	}
	return false
}

type Kind interface {
	Equal(Kind) bool
	String() string
}

type strKind string

func (sk strKind) Equal(oth Kind) bool {
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

func (e KindElement) Equal(oth Kind) bool {
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

type KindValue struct{ Name *string }

func (v KindValue) Equal(oth Kind) bool {
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

var discard = &nullOutput{}

type nullOutput struct{}

func (*nullOutput) Write(p []byte) (n int, err error)        { return len(p), nil }
func (*nullOutput) WriteTo(w io.Writer) (n int64, err error) { return 0, nil }
func (*nullOutput) WriteRune(r rune) (n int, err error)      { return len([]byte(string(r))), nil }
func (*nullOutput) WriteByte(c byte) error                   { return nil }
func (*nullOutput) Bytes() []byte                            { return []byte{} }

var _ noDiscard = objectKeyBuffer{}

type objectKeyBuffer struct {
	bytes.Buffer
	// Due to the requirements for matching the JSON object value path,
	// we need to temporarily store the key value in memory to construct the JSON selector path.
	noDiscard
}

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

	dec *json.Decoder

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
		c.dec = json.NewDecoder(c.Input)
	}

	if !c.inList {
		tkn, err := c.dec.Token()
		if err != nil {
			c.err = err
			return false
		}
		delim, ok := tkn.(json.Delim)
		if !ok {
			c.err = LexingError{Path: []Kind{KindArray},
				Message: fmt.Sprintf("unexpecte json token: %v", tkn)}
			return false
		}
		if delim != '[' {
			c.err = LexingError{
				Message: fmt.Sprintf(`unexpecte json token delimiter, expected "%c" but got "%s"`, '[', delim),
				Path:    []Kind{KindArray},
			}
			return false
		}
		c.inList = true
		c.index = 0
	}

	if !c.dec.More() {
		tkn, err := c.dec.Token()
		if err != nil {
			c.err = err
		}

		delim, ok := tkn.(json.Delim)
		if !ok {
			c.err = LexingError{
				Message: fmt.Sprintf("unexpecte json token: %v", tkn),
				Path:    []Kind{KindArray, KindElement{Index: &c.index}},
			}
			return false
		}
		if delim != ']' {
			c.err = LexingError{
				Message: fmt.Sprintf(`unexpecte json token delimiter, expected "%c" but got "%s"`, ']', delim),
				Path:    []Kind{KindArray, KindElement{Index: &c.index}},
			}
			return false
		}

		c.done = true
		return false
	}
	c.index++

	var raw json.RawMessage
	if err := c.dec.Decode(&raw); err != nil {
		c.err = err
		return false
	}

	c.data = raw
	return true
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

func TrimSpace(data []byte) []byte {
	in := bufio.NewReader(bytes.NewReader(data))
	out := &bytes.Buffer{}
trim:
	for {
		if err := trimSpace(in, discard); err != nil {
			break trim
		}
	move:
		for { // move until next white space
			_, _, err := iokit.MoveRune(in, out)
			if err != nil {
				break trim
			}

			char, _, err := iokit.PeekRune(in)
			if err != nil {
				break trim
			}

			if _, ok := whitespaceChars[char]; ok {
				break move
			}
		}
	}
	return out.Bytes()
}

// ScanFrom is a syntax sugar to use Scan with string and byte slices
func ScanFrom[T string | []byte | *bufio.Reader](v T) (json.RawMessage, error) {
	switch src := any(v).(type) {
	case string:
		return Scan(bufio.NewReader(strings.NewReader(src)))
	case []byte:
		return Scan(bufio.NewReader(bytes.NewReader(src)))
	case *bufio.Reader:
		return Scan(src)
	default:
		panic("not-implemented")
	}
}

// Scan scan a json raw message out from an input source.
func Scan(in Input) (json.RawMessage, error) {
	var output json.RawMessage
	var s = Scanner{
		Selectors: []Selector{{
			Func: func(data json.RawMessage) error {
				output = data
				return nil
			},
		}},
	}
	err := s.Scan(in)
	return output, err
}
