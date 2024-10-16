package jsontoken

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
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
	I *bufio.Reader

	buf  bytes.Buffer
	last bytes.Buffer

	path []Kind

	prev struct {
		kind  Kind
		token []byte
	}
}

func ScanFrom[T string | []byte | *bufio.Reader](v T) (json.RawMessage, error) {
	var s Scanner
	switch src := any(v).(type) {
	case string:
		return s.Scan(bufio.NewReader(strings.NewReader(src)))
	case []byte:
		return s.Scan(bufio.NewReader(bytes.NewReader(src)))
	case *bufio.Reader:
		return s.Scan(src)
	default:
		panic("not-implemented")
	}
}

func Scan(b *bufio.Reader) (json.RawMessage, error) {
	var s = &Scanner{I: b}
	return s.Scan(b)
}

func (s *Scanner) Scan(in *bufio.Reader) (json.RawMessage, error) {
	var out = &bytes.Buffer{}
	err := s.scan(in, out)
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

func (s *Scanner) scan(in *bufio.Reader, out *bytes.Buffer) error {
	if err := trimSpace(in, out); err != nil {
		return err
	}

	char, _, err := peekRune(in)
	if err != nil {
		return err
	}

	switch s.tokenStartKind(char) {
	case KindNull:
		return s.scanNull(in, out)
	case KindBoolean:
		return s.scanBoolean(in, out)
	case KindString:
		return s.scanString(in, out)
	case KindArray:
		return s.scanArray(in, out)
	case KindObject:
		return s.scanObject(in, out)
	case KindNumber: // detecting the end of the number takes a different approach than the rest of the json kinds.
		return s.scanNumber(in, out)
	}
	return nil
}

func peekRune(in *bufio.Reader) (rune, int, error) {
	char, size, err := in.ReadRune()
	if err != nil {
		return char, size, err
	}
	if err := in.UnreadRune(); err != nil {
		return char, size, err
	}
	return char, size, err
}

func moveRune(in *bufio.Reader, out *bytes.Buffer) (rune, int, error) {
	char, size, err := in.ReadRune()
	if err != nil {
		return char, size, err
	}
	if _, err := out.WriteRune(char); err != nil {
		return 0, 0, err
	}
	return char, size, nil
}

func trimSpace(in *bufio.Reader, out *bytes.Buffer) error {
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

func (s *Scanner) scanNumber(in *bufio.Reader, out *bytes.Buffer) error {
	for {
		digit, _, err := in.ReadRune()
		if err != nil {
			return err
		}
		if _, ok := numberChars[digit]; !ok {
			// no more number chars, we are ready,
			// the last read should be reverted.
			return in.UnreadRune()
		}
		if _, err := out.WriteRune(digit); err != nil {
			return err
		}
	}
}

func (s *Scanner) scanNull(in *bufio.Reader, out *bytes.Buffer) error {
	return s.scanToken(in, out, nullToken)
}

func (s *Scanner) scanToken(in *bufio.Reader, out *bytes.Buffer, token []rune) error {
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
}

func (s *Scanner) scanBoolean(in *bufio.Reader, out *bytes.Buffer) error {
	if err := trimSpace(in, out); err != nil {
		return err
	}
	char, _, err := peekRune(in)
	if err != nil {
		return err
	}
	switch char {
	case 't':
		return s.scanToken(in, out, trueToken)
	case 'f':
		return s.scanToken(in, out, falseToken)
	default:
		return s.malformedF("unexpected boolean first character: %c", char)
	}
}

func (s *Scanner) scanArray(in *bufio.Reader, out *bytes.Buffer) error {
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
		_, err := out.WriteRune(nextChar)
		return err
	}

scanValues:
	for {
		if err := trimSpace(in, out); err != nil {
			return err
		}
		// scan array value
		err := s.scan(in, out)
		if err != nil {
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
}

func (s *Scanner) scanObject(in *bufio.Reader, out *bytes.Buffer) error {
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
		// scan string key
		err = s.scanString(in, out)
		if err != nil {
			return fmt.Errorf("(object key) %w", err)
		}

		if err := trimSpace(in, out); err != nil {
			return err
		}

		// read value sep
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

		// scan value
		err = s.scan(in, out)
		if err != nil {
			return err
		}

		if err := trimSpace(in, out); err != nil {
			return err
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
}

func (s *Scanner) scanString(in *bufio.Reader, out *bytes.Buffer) error {
	var buf bytes.Buffer
	char, _, err := in.ReadRune()
	if err != nil {
		return err
	}
	if char != quoteToken {
		return s.malformedF(`unexpected string starting token, expected quote but got "%c"`, char)
	}
	if _, err := buf.WriteRune(char); err != nil {
		return err
	}
scan:
	for {
		char, _, err := in.ReadRune()
		if err != nil {
			return err
		}
		if _, err := buf.WriteRune(char); err != nil {
			return err
		}
		if char == quoteToken {
			// it is only enough to check if the string is fully found when we see a potential closing quote character.
			// this way, we don't need to check the validity on each utf8 character.
			if json.Valid(buf.Bytes()) {
				break scan
			}
		}
	}
	for _, c := range buf.Bytes() {
		if err := out.WriteByte(c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) tokenStartKind(char rune) Kind {
	if _, ok := numberChars[char]; ok {
		return KindNumber
	}
	if _, ok := whitespaceChars[char]; ok {
		return Whitespace
	}
	switch char {
	case '[':
		return KindArray

	// case ']':
	// 	return s.popKind(KindArray, "]", char)

	case '{':
		return KindObject

	// case '}':
	// 	return s.popKind(KindObject, "}", char)

	case '"':
		return KindString

	case 'n':
		return KindNull

	case 't', 'f':
		return KindBoolean

	}
	return NilKind
}

func (s *Scanner) isNextStringCharEscaped(cur *bytes.Buffer) bool {
	var escape bool
	var offset int
	var token []byte = cur.Bytes()
	for {
		if !(offset < len(token)) {
			break
		}

		last := token[:len(token)-offset]

		char, size := utf8.DecodeLastRune(last)
		offset += size

		if char == '\\' {
			escape = !escape
		} else {
			break
		}
	}
	return escape
}

func (s *Scanner) malformedF(format string, a ...any) error {
	args := []any{ErrMalformed}
	args = append(args, a...)
	return fmt.Errorf("[%w] "+format, args...)
}

func (s *Scanner) malformedErr(err error) error {
	return s.malformedF("%w", err)
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
}

/* KIND */

type Kind string

const (
	NilKind     Kind = ""
	Whitespace  Kind = "whitespace"
	KindArray   Kind = "array"
	KindObject  Kind = "object"
	KindString  Kind = "string"
	KindNumber  Kind = "number"
	KindBoolean Kind = "boolean"
	KindNull    Kind = "null"
)

var _ = enum.Register[Kind](
	NilKind,
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
