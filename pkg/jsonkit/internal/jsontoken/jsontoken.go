package jsontoken

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/iterators"
)

const ErrMalformed errorkit.Error = "malformed json error"

var (
	nullToken  = []rune("null")
	trueToken  = []rune("true")
	falseToken = []rune("false")
	quoteToken = '"'

	arrayOpenToken  = '['
	arrayCloseToken = ']'
	valueSepToken   = ','

	objectOpenToken  = '{'
	objectCloseToken = '}'
	nameSepToken     = ':'
)

type Scanner struct {
	Path Path
	Func func(json.RawMessage) error

	noDiscard bool
}

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

type Input interface {
	ReadRune() (r rune, size int, err error)
	UnreadRune() error
}

type Output interface {
	io.Writer
	WriteTo(w io.Writer) (n int64, err error)
	WriteRune(r rune) (n int, err error)
	WriteByte(c byte) error
	Bytes() []byte
}

func Scan(b *bufio.Reader) (json.RawMessage, error) {
	var s Scanner
	return s.Scan(b, nil)
}

func (s *Scanner) Scan(in Input, path Path) (json.RawMessage, error) {
	var out bytes.Buffer
	err := s.scan(in, &out, path)
	if err == io.EOF {
		if json.Valid(out.Bytes()) {
			return out.Bytes(), nil
		}
		return nil, s.malformedErr(err)
	}
	if err != nil {
		return nil, s.malformedErr(err)
	}
	if !json.Valid(out.Bytes()) {
		return nil, ErrMalformed
	}
	return out.Bytes(), nil
}

func (s *Scanner) scan(in Input, out Output, path Path) error {
	if in == nil {
		return nil
	}
	if err := trimSpace(in, out); err != nil {
		return err
	}
	char, _, err := peekRune(in)
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
		return fmt.Errorf("not-implemented, unable how to handle %s kind", kind)
	}
}

func (s *Scanner) with(out Output, path Path, blk func(out Output) error) error {
	var raw Output = &bytes.Buffer{}
	pathMatches := s.Path.Match(path)
	if !pathMatches && !s.noDiscard {
		raw = discard
	}
	returnErr := blk(raw)
	if returnErr != nil && !errors.Is(returnErr, io.EOF) { // EOF is a good type of error, signaling the end of the input stream
		return returnErr
	}
	if pathMatches && s.Func != nil {
		if err := s.Func(raw.Bytes()); err != nil {
			return err
		}
	}
	if _, err := raw.WriteTo(out); err != nil {
		return err
	}
	return returnErr
}

func peekRune(in Input) (rune, int, error) {
	char, size, err := in.ReadRune()
	if err != nil {
		return char, size, err
	}
	if err := in.UnreadRune(); err != nil {
		return char, size, err
	}
	return char, size, err
}

func moveRune(in Input, out Output) (rune, int, error) {
	char, size, err := in.ReadRune()
	if err != nil {
		return char, size, err
	}
	if _, err := out.WriteRune(char); err != nil {
		return 0, 0, err
	}
	return char, size, nil
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
	return s.scanToken(in, out, path.With(KindNull), nullToken)
}

func (s *Scanner) scanToken(in Input, out Output, path Path, token []rune) error {
	return s.with(out, path, func(out Output) error {
		for i := 0; i < len(token); i++ {
			char, _, err := in.ReadRune()
			if err != nil {
				return s.malformedF("error while parsing %q token: %w", string(token), err)
			}
			if char != token[i] {
				return s.malformedF(`error parsing %q token: expected "%q" but got "%c"`, string(token), char, token[i])
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
	char, _, err := peekRune(in)
	if err != nil {
		return err
	}
	switch char {
	case 't':
		return s.scanToken(in, out, path, trueToken)
	case 'f':
		return s.scanToken(in, out, path, falseToken)
	default:
		return s.malformedF("unexpected boolean first character: %c", char)
	}
}

func (s *Scanner) scanArray(in Input, out Output, path Path) error {
	path = path.With(KindArray)
	return s.with(out, path, func(out Output) error {
		if err := trimSpace(in, out); err != nil {
			return err
		}
		char, _, err := moveRune(in, out)
		if err != nil {
			return err
		}
		if char != arrayOpenToken {
			return s.malformedF(`unexpected array open token, expected "[" but got %c`, char)
		}

		nextChar, _, err := peekRune(in)
		if err != nil {
			return err
		}
		if nextChar == arrayCloseToken { // empty array
			_, _, err := moveRune(in, out)
			return err
		}

	scanValues:
		for {
			if err := trimSpace(in, out); err != nil {
				return err
			}

			// scan array value
			if err := s.scan(in, out, path.With(KindArrayValue)); err != nil {
				return err
			}

			if err := trimSpace(in, out); err != nil {
				return err
			}

			// scan sep/close
			next, _, err := moveRune(in, out)
			if err != nil {
				return err
			}
			switch next {
			case valueSepToken: // has more
				continue scanValues
			case arrayCloseToken:
				break scanValues
			default:
				return s.malformedF("unexpected array token: %c", next)
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

		firstChar, _, err := moveRune(in, out)
		if err != nil {
			return err
		}

		if firstChar != objectOpenToken { // '{'
			return s.malformedF(`unexpected object open token, expected "{" but got %c`, firstChar)
		}

		if err := trimSpace(in, out); err != nil {
			return err
		}

		secondChar, _, err := peekRune(in)
		if err != nil {
			return err
		}
		if secondChar == objectCloseToken { // empty object
			_, _, err := moveRune(in, out) // write '}'
			return err
		}

	scan:
		for {
			if err := trimSpace(in, out); err != nil {
				return err
			}

			{ /* key-value pair */

				/* SCAN STRING KEY */

				// we need to make sure that the object key is retrieved
				// and not discarded from the output writing.
				s.noDiscard = true
				var key bytes.Buffer
				err := s.scanString(in, &key, path.With(KindObjectKey))
				s.noDiscard = false
				if err != nil {
					return fmt.Errorf("(object key) %w", err)
				}

				if err := copyTo(&key, out); err != nil {
					return err
				}

				if err := trimSpace(in, out); err != nil {
					return err
				}

				/* SEPERATOR */
				sep, _, err := moveRune(in, out)
				if err != nil {
					return err
				}
				if sep != nameSepToken {
					return s.malformedF(`unexpected object key-value separator, expected ":" but got "%c"`, sep)
				}
				if err := trimSpace(in, out); err != nil {
					return err
				}

				/* SCAN OBJECT VALUE */
				if err := s.scan(in, out, path.With(KindObjectValue{Key: key.Bytes()})); err != nil {
					return err
				}
				if err := trimSpace(in, out); err != nil {
					return err
				}
			}

			next, _, err := moveRune(in, out)
			if err != nil {
				return err
			}
			switch next {
			case objectCloseToken:
				return nil
			case valueSepToken:
				continue scan
			default:
				return s.malformedF(`unexpected character in object, expected either "," or "}", but got "%c"`, next)
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
		first, _, err := moveRune(in, &str)
		if err != nil {
			return err
		}
		if first != quoteToken {
			return s.malformedF(`unexpected string starting token, expected quote but got "%c"`, first)
		}
	scan:
		for {
			char, _, err := moveRune(in, &str)
			if err != nil {
				return err
			}
			if char == quoteToken {
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
		return whitespace
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
	return nilKind
}

func (s *Scanner) malformedF(format string, a ...any) error {
	args := []any{ErrMalformed}
	args = append(args, a...)
	return fmt.Errorf("[%w] "+format, args...)
}

func (s *Scanner) malformedErr(err error) error {
	return s.malformedF("%w", err)
}

func CD(ctx context.Context, in Input, path Path) *Visitor {
	return &Visitor{
		Context: ctx,
		Input:   in,
		Path:    path,
	}
}

type Visitor struct {
	Context context.Context
	Input   Input
	Path    Path

	init sync.Once

	m   sync.RWMutex
	out *iterators.PipeOut[json.RawMessage]

	raw json.RawMessage
	err error
}

func (v *Visitor) Next() bool {
	if v.err != nil {
		return false
	}

	v.init.Do(func() {
		v.m.Lock()
		defer v.m.Unlock()
		in, out := iterators.PipeWithContext[json.RawMessage](v.Context)
		v.out = out
		go func() {
			defer in.Close()
			const breakScanning errorkit.Error = "break"
			sc := Scanner{
				Path: v.Path,
				Func: func(rm json.RawMessage) error {
					if in.Value(rm) {
						return nil
					}
					return breakScanning
				},
			}
			_, err := sc.Scan(v.Input, nil)
			if errors.Is(err, breakScanning) {
				return
			}
			in.Error(err)
		}()
	})

	if v.Context.Err() != nil {
		return false
	}

	v.m.RLock()
	defer v.m.RUnlock()
	if v.out.Next() {
		v.raw = v.out.Value()
		return true
	}
	return false
}

func (v *Visitor) Value() json.RawMessage { return v.raw }
func (v *Visitor) Err() error             { return v.err }

func (v *Visitor) Close() error {
	v.m.Lock()
	defer v.m.Unlock()
	var outErr error
	if v.out != nil {
		outErr = v.out.Close()
	}
	var inputErr error
	if c, ok := v.Input.(io.Closer); ok {
		inputErr = c.Close()
	}
	return errorkit.Merge(outErr, inputErr)
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

func (p Path) String() string {
	return strings.Join(slicekit.Map(p, Kind.String), ".")
}

func (p Path) With(k Kind) Path {
	return append(slicekit.Clone(p), k)
}

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
	nilKind    strKind = ""
	whitespace strKind = "whitespace"

	KindArray      strKind = "array"
	KindArrayValue strKind = "array-value"

	KindObject    strKind = "object"
	KindObjectKey strKind = "object-key"

	KindString  strKind = "string"
	KindNumber  strKind = "number"
	KindBoolean strKind = "boolean"
	KindNull    strKind = "null"
)

type KindObjectValue struct {
	// Key is the raw json data that represents the Key value
	Key json.RawMessage
}

func (k KindObjectValue) Equal(oth Kind) bool {
	other, ok := oth.(KindObjectValue)
	if !ok {
		return false
	}
	if len(k.Key) == 0 {
		return true
	}
	if len(k.Key) != len(other.Key) {
		return false
	}
	for i := 0; i < len(k.Key); i++ {
		if k.Key[i] != other.Key[i] {
			return false
		}
	}
	return true
}

func (k KindObjectValue) String() string {
	var name = "object-value"
	if len(k.Key) != 0 {
		name = fmt.Sprintf("%s(key=%s)", name, string(k.Key))
	}
	return name
}

var _ = enum.Register[Kind](
	nilKind,
	KindArray,
	KindObject,
	KindString,
	KindNumber,
	KindBoolean,
	KindNull,
)

func IterateArray(r io.Reader) *ArrayIterator {
	return &ArrayIterator{I: r}
}

type ArrayIterator struct {
	I io.Reader

	buf *bufio.Reader
	err error
	raw json.RawMessage

	in bool
	dn bool
}

func (i *ArrayIterator) Close() error {
	if c, ok := i.I.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (i *ArrayIterator) Err() error {
	return i.err
}

func (i *ArrayIterator) Next() bool {
	if i.err != nil {
		return false
	}
	if i.dn {
		return false
	}
	if i.buf == nil {
		i.buf = bufio.NewReader(i.I)
	}
	if err := trimSpace(i.buf, &bytes.Buffer{}); err != nil {
		i.err = err
		return false
	}
	char, _, err := i.buf.ReadRune()
	if err != nil {
		i.err = err
		return false
	}
	switch char {
	case '[':
		if i.in {
			i.err = fmt.Errorf("%w: unexpected %c character expected [", ErrMalformed, char)
			return false
		}
		i.in = true

	case ',': // has more
		break

	case ']':
		i.dn = true
		return false

	default:
		i.err = fmt.Errorf(`%w: unexpected %c character expected one of: "[" / "]" / ","`, ErrMalformed, char)
		return false
	}

	i.raw, err = Scan(i.buf)
	if err != nil {
		i.err = err
		return false
	}

	return true
}

func (i *ArrayIterator) Value() json.RawMessage {
	return i.raw
}

var discard = &nullOutput{}

type nullOutput struct{}

func (*nullOutput) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (*nullOutput) WriteTo(w io.Writer) (n int64, err error) {
	return 0, nil
}

func (*nullOutput) WriteRune(r rune) (n int, err error) {
	return len([]byte(string(r))), nil
}

func (*nullOutput) WriteByte(c byte) error {
	return nil
}

func (*nullOutput) Bytes() []byte {
	return []byte{}
}
