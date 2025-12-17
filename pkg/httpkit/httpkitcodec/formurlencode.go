package httpkitcodec

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/refnode"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase/pp"
)

type FormURLEncoded[T any] struct {
	// Collection is the id/name of the collection type.
	// The default value is the short type name of T in snake case with an "s" suffix.
	// For example if T is
	// - `Item` -> `items`
	// - `User` -> `users`
	// - `UserEmail` -> `user_emails`
	Collection string
	// StringCase is the formatter used to format the url keys.
	// Default: stringkit.ToSnake
	StringCase func(string) string
	prefix     string

	stream bool
}

func (c FormURLEncoded[T]) withPrefix(prefix string) FormURLEncoded[T] {
	if len(c.prefix) == 0 {
		c.prefix = prefix
		return c
	}
	c.prefix = c.prefix + "." + prefix
	return c
}

func (c *FormURLEncoded[T]) fmtKey(key string) string {
	if c.StringCase != nil {
		return c.StringCase(key)
	}
	return stringkit.ToSnake(key)
}

var pkgPrefix = regexp.MustCompile(`^[\w\d]+\.`)

func (c *FormURLEncoded[T]) typeName(typ reflect.Type) string {
	if name := typ.Name(); 0 < len(name) {
		return c.fmtKey(name)
	}
	raw := typ.String()
	raw = pkgPrefix.ReplaceAllString(raw, "")
	return c.fmtKey(raw)
}

func (c *FormURLEncoded[T]) getCollection() string {
	if len(c.Collection) == 0 {
		c.Collection = c.typeName(reflectkit.TypeOf[T]()) + "s"
	}
	return c.Collection
}

var mediaTypeFormURLEncoded = map[string]struct{}{
	mediatype.FormUrlencoded: {},
}

func (c FormURLEncoded[T]) SupporsMediaType(mediaType string) bool {
	_, ok := mediaTypeFormURLEncoded[mediaType]
	return ok
}

func (c FormURLEncoded[T]) filterValues(values url.Values) url.Values {
	if 0 < len(c.prefix) {
		var vs = url.Values{}
		for key, value := range values {
			if !strings.HasPrefix(key, c.prefix) {
				continue
			}
			vs[strings.TrimSuffix(key, c.prefix)] = value
		}
		return vs
	}
	return values
}

func (c FormURLEncoded[T]) formatValues(values url.Values) {
	if 0 < len(c.prefix) {
		var addSuffix func(vs url.Values, key, suffix string)
		addSuffix = func(vs url.Values, key, suffix string) {
			if vs == nil {
				return
			}
			nkey := key + suffix
			if _, ok := vs[nkey]; ok {
				addSuffix(vs, nkey, suffix)
			}
			vs[nkey] = vs[key]
			delete(vs, key)
		}
		for _, key := range mapkit.Keys(values) {
			addSuffix(values, key, c.prefix)
		}
	}
}

func (c FormURLEncoded[T]) Marshal(v T) ([]byte, error) {
	var input = reflect.ValueOf(v)
	q := url.Values{}
	err := c.marshalAppend(q, "", input)
	return []byte(q.Encode()), err
}

func (c FormURLEncoded[T]) marshalAppend(vs url.Values, qKey string, val reflect.Value) error {
	switch val.Kind() {
	case reflect.Struct:
		c := c.withPrefix(qKey)
		for i, num := 0, val.NumField(); i < num; i++ {
			var (
				typ  = val.Type().Field(i)
				val  = val.Field(i)
				prop = c.getFormProperties(typ)
			)
			if prop.OmitEmpty && reflectkit.IsEmpty(val) {
				continue
			}
			if err := c.marshalAppend(vs, c.qKeyFor(prop.Name), val); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		var m = val
		for _, mKey := range m.MapKeys() {
			mVal := m.MapIndex(mKey)
			mKeyStr, err := convkit.FormatReflect(mKey)
			if err != nil {
				return fmt.Errorf("error while formatting %#v key: %w", mKey.Interface(), err)
			}

			if err := c.marshalAppend(vs, c.qKeyFor(mKeyStr), mVal); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		for i, l := 0, val.Len(); i < l; i++ {
			value, err := convkit.Format(val.Index(i))
			if err != nil {
				return err
			}
			vs.Add(qKey, value)
		}
		return nil
	default:
		qVal, err := convkit.FormatReflect(val)
		if err != nil {
			return err
		}
		vs.Add(qKey, qVal)
		return nil
	}
}

func (c FormURLEncoded[T]) Unmarshal(data []byte, p *T) error {
	vs, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	return c.unmarshal(vs, p)
}

func (c FormURLEncoded[T]) unmarshal(vs url.Values, p *T) error {
	pp.PP(vs)
	if p == nil {
		return fmt.Errorf("nil pointer received")
	}
	var ptr = reflect.ValueOf(p)

	var qKeyOf = func(v reflectkit.V) (string, error) {
		var k []string
		for e := range v.Iter() {
			switch e.NodeType {
			case refnode.ArrayElem, refnode.SliceElem:
				k = append(k, strconv.Itoa(e.Index))
			case refnode.StructField:
				prop := c.getFormProperties(e.StructField)
				k = append(k, prop.Name)
			case refnode.MapValue:
				mKeyStr, err := convkit.FormatReflect(e.MapKey)
				if err != nil {
					return "", err
				}
				k = append(k, mKeyStr)
			}
		}
		return strings.Join(k, "."), nil
	}

	for v := range reflectkit.Visit(ptr) {
		qKey, err := qKeyOf(v)
		if err != nil {
			return err
		}
		pp.PP(qKey, v.Value.Interface())
		vs, ok := vs[qKey]
		pp.PP(vs, ok, v.Value.Interface())
	}

	// switch ptr.Type().Elem().Kind() {
	// case reflect.Struct:
	// 	return c.unmarshalStruct(vs, ptr)
	// case reflect.Map:
	// 	return c.unmarshalMap(vs, ptr)
	// default:
	return fmt.Errorf("not implemented type: %s", ptr.Type().Elem().String())
	// }
}

func (c FormURLEncoded[T]) unmarshalStruct(vs url.Values, ptr reflect.Value) error {
	for i, num := 0, ptr.Type().Elem().NumField(); i < num; i++ {
		var (
			field = ptr.Elem().Field(i)
			props = c.getFormProperties(ptr.Elem().Type().Field(i))
		)
		switch field.Type().Kind() {
		case reflect.Slice:
			list := reflect.MakeSlice(field.Type(), 0, len(vs[props.Name]))
			for _, queryValue := range vs[props.Name] {
				out, err := convkit.ParseReflect(field.Type(), queryValue)
				if err != nil {
					return err
				}
				list = reflect.Append(list, out)
			}
			field.Set(list)

		default:
			out, err := convkit.ParseReflect(field.Type(), vs.Get(props.Name))
			if err != nil {
				return err
			}
			field.Set(out)

		}
	}
	return nil
}

func (c FormURLEncoded[T]) unmarshalMap(values url.Values, ptr reflect.Value) error {
	var (
		keyType   = ptr.Type().Elem().Key()
		valueType = ptr.Type().Elem().Elem()
	)
	ptr.Elem().Set(reflect.MakeMap(ptr.Type().Elem())) // create a new map[K]V
	for qKey, qVS := range values {
		out, err := slicekit.MapErr[any](qVS, func(i string) (any, error) {
			return convkit.DuckParse[string](i)
		})
		if err != nil {
			return fmt.Errorf("error while parsing %s's values: %w", qKey, err)
		}

		mKey, err := convkit.ParseReflect(keyType, qKey)
		if err != nil {
			return fmt.Errorf("failed to parse key value for type %s: %w",
				keyType.String(), err)
		}

		var mVal reflect.Value
		switch {
		case len(out) == 0:
			continue
		case len(out) == 1:
			mVal = reflect.ValueOf(out[0])
		case 0 < len(out):
			mVal = reflect.ValueOf(out)
		}
		if !mVal.CanConvert(valueType) {
			return fmt.Errorf("map key is incompatible with the parsed value type at key %s: expected %s but got %s",
				qKey, valueType.String(), mVal.Type().String())
		}
		ptr.Elem().SetMapIndex(mKey, mVal.Convert(valueType))
	}
	return nil
}

type formProperties struct {
	Name      string
	OmitEmpty bool
}

func (c FormURLEncoded[T]) qKeyFor(k string) string {
	if len(c.prefix) != 0 {
		k = c.prefix + "." + k
	}
	return k
}

func (c FormURLEncoded[T]) getFormProperties(typ reflect.StructField) formProperties {
	var prop formProperties
	prop.Name = c.fmtKey(typ.Name)
	c.lookupTag(typ, "url", &prop)
	c.lookupTag(typ, "form", &prop)
	return prop
}

func (c FormURLEncoded[T]) lookupTag(typ reflect.StructField, tagKey string, prop *formProperties) {
	tag, ok := typ.Tag.Lookup(tagKey)
	if !ok || len(tag) == 0 {
		return
	}
	parts := strings.Split(tag, ",")
	prop.Name = parts[0]
	if 1 < len(parts) {
		for _, part := range parts[1:] {
			switch strings.TrimSpace(part) {
			case "omitempty":
				prop.OmitEmpty = true
			}
		}
	}
}

func (c FormURLEncoded[T]) NewListEncoder(w io.Writer) codec.StreamEncoder[T] {
	return &formURLStreamEncoder[T]{FormURLEncoded: c, Writer: w}
}

type formURLStreamEncoder[T any] struct {
	FormURLEncoded[T]
	Writer io.Writer
	index  int
}

func (se *formURLStreamEncoder[T]) Encode(v T) error {
	enc := se.FormURLEncoded.
		withPrefix(se.FormURLEncoded.getCollection()).
		withPrefix(strconv.Itoa(se.index))

	data, err := enc.Marshal(v)
	if err != nil {
		return err
	}
	if 0 < se.index {
		se.Writer.Write([]byte("&"))
	}
	_, err = iokit.WriteAll(se.Writer, data)
	se.index++
	if err != nil {
		return err
	}

	return nil
}

func (se *formURLStreamEncoder[T]) Close() error {
	return nil
}

func (c FormURLEncoded[T]) NewListDecoder(r io.Reader) codec.StreamDecoder[T] {
	return iterkit.FromPullIter(&formURLStreamDecoder[T]{FormURLEncoded: c, Reader: r})
}

type formURLStreamDecoder[T any] struct {
	FormURLEncoded[T]
	Reader io.Reader

	index int
	err   error
	done  bool

	buffer *bufio.Reader

	curQuery  url.Values
	curSuffix string

	queryBuffer url.Values
}

func (c *formURLStreamDecoder[T]) Err() error {
	return c.err
}

func (c *formURLStreamDecoder[T]) Next() bool {
	if c.done {
		return false
	}
	if c.err != nil {
		return false
	}

	if c.buffer == nil {
		c.buffer = bufio.NewReader(c.Reader)
	}

	c.curSuffix = fmt.Sprintf("%s[%d]", c.FormURLEncoded.getCollection(), c.index)
	c.index++

	var prev = url.Values{}
	if 0 < len(c.queryBuffer) {
		for key, kvs := range c.queryBuffer {
			if strings.HasPrefix(key, c.curSuffix) {
				prev[key] = kvs
				delete(c.queryBuffer, key)
			}
		}
	}

	var contSignature = []byte(url.QueryEscape(c.curSuffix))

	var queryPart []byte
	for {
		part, err := c.buffer.ReadBytes('&')
		if 0 < len(part) {
			queryPart = append(queryPart, part...)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				c.err = err
				return false
			}
		}
		if !bytes.Contains(part, contSignature) {
			break
		}
	}
	if len(queryPart) == 0 {
		c.done = true
		return false
	}
	bytes.TrimSuffix(queryPart, []byte("&"))

	query, err := url.ParseQuery(string(queryPart))
	if err != nil {
		c.err = err
		return false
	}

	query = mapkit.Merge(query, prev)

	for key, vs := range query {
		if !strings.HasPrefix(key, c.curSuffix) {
			if c.queryBuffer == nil {
				c.queryBuffer = make(url.Values)
			}
			c.queryBuffer[key] = vs
			delete(query, key)
		}
	}

	c.curQuery = query
	return true
}

func (c *formURLStreamDecoder[T]) Value() codec.Decoder[T] {
	return c
}

func (c *formURLStreamDecoder[T]) Decode(p *T) error {
	dec := c.FormURLEncoded
	dec.prefix = c.curSuffix
	return dec.unmarshal(c.curQuery, p)
}

func (c *formURLStreamDecoder[T]) Close() error {
	if len(c.queryBuffer) != 0 {
		return fmt.Errorf("unprocessed query string are in the stream decoder:\n%s", c.queryBuffer.Encode())
	}
	return nil
}
