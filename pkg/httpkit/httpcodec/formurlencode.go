package httpcodec

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
	"go.llib.dev/frameless/pkg/reflectkit/refvis"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/testcase/pp"
)

func FormURLEncoded[T any]() Codec[T] {
	var c formURLEncoded[T]
	return Codec[T]{
		Marshal:   c.Marshal,
		Unmarshal: c.Unmarshal,

		List: ListCodec[T]{},
	}
}

type formURLEncoded[T any] struct {
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
}

func (c formURLEncoded[T]) withPrefix(prefix string) formURLEncoded[T] {
	if len(c.prefix) == 0 {
		c.prefix = prefix
		return c
	}
	c.prefix = c.prefix + "." + prefix
	return c
}

func (c *formURLEncoded[T]) fmtKey(key string) string {
	if c.StringCase != nil {
		return c.StringCase(key)
	}
	return stringkit.ToSnake(key)
}

var pkgPrefix = regexp.MustCompile(`^[\w\d]+\.`)

func (c *formURLEncoded[T]) typeName(typ reflect.Type) string {
	if name := typ.Name(); 0 < len(name) {
		return c.fmtKey(name)
	}
	raw := typ.String()
	raw = pkgPrefix.ReplaceAllString(raw, "")
	return c.fmtKey(raw)
}

func (c *formURLEncoded[T]) getCollection() string {
	if len(c.Collection) == 0 {
		c.Collection = c.typeName(reflectkit.TypeOf[T]()) + "s"
	}
	return c.Collection
}

var mediaTypeFormURLEncoded = map[string]struct{}{
	mediatype.FormUrlencoded: {},
}

func (c formURLEncoded[T]) filterValues(values url.Values) url.Values {
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

func (c formURLEncoded[T]) formatValues(values url.Values) {
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

func (c formURLEncoded[T]) Marshal(v T) ([]byte, error) {
	var input = reflect.ValueOf(v)
	q := url.Values{}
	err := c.marshalAppend(q, "", input)
	return []byte(q.Encode()), err
}

func (c formURLEncoded[T]) marshalAppend(vs url.Values, qKey string, val reflect.Value) error {
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
			if err := c.marshalAppend(vs, strconv.Itoa(i), val.Index(i)); err != nil {
				return err
			}
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

func (c formURLEncoded[T]) Unmarshal(data []byte, p *T) error {
	vs, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("nil pointer received")
	}

	return c.unmarshal(vs, reflect.ValueOf(p))
}

func (c formURLEncoded[T]) getQueryKeyFor(node refvis.Node) (string, error) {
	var k []string
	for e := range node.Iter() {
		switch e.Type {
		case refvis.ArrayElem, refvis.SliceElem:
			k = append(k, strconv.Itoa(e.Index))
		case refvis.StructField:
			prop := c.getFormProperties(e.StructField)
			k = append(k, prop.Name)
		case refvis.MapValue:
			mKeyStr, err := convkit.FormatReflect(e.MapKey)
			if err != nil {
				return "", err
			}
			k = append(k, mKeyStr)
		}
	}
	return strings.Join(k, "."), nil
}

func (c formURLEncoded[T]) getQueryFor(q url.Values, n refvis.Node) (url.Values, error) {
	prefix, err := c.getQueryKeyFor(n)
	if err != nil {
		return nil, err
	}
	var subq url.Values = make(url.Values)
	for k, vs := range q {
		if strings.HasPrefix(k, prefix) {
			subq[k] = vs
		}
	}
	return subq, nil
}

func (c formURLEncoded[T]) getQueryKeyValue(query url.Values, node refvis.Node) ([]string, bool, error) {
	qKey, err := c.getQueryKeyFor(node)
	if err != nil {
		return nil, false, err
	}
	qVS, ok := query[qKey]
	if !ok {
		return nil, false, nil
	}
	return qVS, true, nil
}

func (c formURLEncoded[T]) unmarshal(q url.Values, val reflect.Value) error {
	return refvis.Walk(val.Addr(), func(n refvis.Node) error {
		switch n.Type {
		case refvis.Map:
			sq, err := c.getQueryFor(q, n)
			if err != nil {
				return err
			}
			if len(sq) == 0 {
				return refvis.StepOver
			}

			// n.Value.Set(reflect.MakeMap(n.VType))

			return nil

			if err := c.unmarshalMap(q, n); err != nil {
				return err
			}
			return refvis.StepOver

		case refvis.Slice:
			qVS, ok, err := c.getQueryKeyValue(q, n)
			if err != nil {
				return err
			}
			if !ok {
				return refvis.StepOver
			}
			n.Value.Set(reflect.MakeSlice(n.Value.Type(), len(qVS), len(qVS)))
			fallthrough
		case refvis.Array:
			qVS, ok, err := c.getQueryKeyValue(q, n)
			if err != nil {
				return err
			}
			if !ok {
				return refvis.StepOver
			}
			var elemType = n.Value.Type().Elem()
			for i, raw := range qVS {
				if err := convkit.UnmarshalReflect(elemType, []byte(raw), n.Value.Index(i).Addr()); err != nil {
					return err
				}
			}
			return refvis.StepOver
		default:
			qVS, ok, err := c.getQueryKeyValue(q, n)
			if err != nil {
				return err
			}
			if !ok {
				return refvis.StepOver
			}
			raw, ok := slicekit.First(qVS)
			if !ok {
				return nil
			}

			var typ = n.Value.Type()
			if n.Type == refvis.StructField {
				typ = n.StructField.Type
			}

			if err := convkit.UnmarshalReflect(typ, []byte(raw), n.Value.Addr()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c formURLEncoded[T]) unmarshalStruct(vs url.Values, ptr reflect.Value) error {
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

func (c formURLEncoded[T]) unmarshalValue(query url.Values, n refvis.Node) error {
	return nil
	// vs, ok, err := c.getQueryKeyValue(query, n)
	// if err != nil {
	// 	return err
	// }
	// if !ok {
	// 	return refvis.StepOver
	// }
	// switch n.Value.Kind() {
	// case reflect.Slice:
	// 	// mValP.Elem().Set(reflect.MakeSlice(valueType, len(vs), len(vs)))
	// 	// ptr.Elem().SetMapIndex(mKey, mValP.Elem())

	// case reflect.Array:
	// 	elemType := valueType.Elem()
	// 	for i, raw := range vs {
	// 		err := convkit.UnmarshalReflect(elemType, []byte(raw), mValP.Elem().Index(i).Addr())
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// default:
	// 	raw, ok := slicekit.Last(vs)
	// 	if !ok {
	// 		continue
	// 	}
	// 	err := convkit.UnmarshalReflect(valueType, []byte(raw), mValP)
	// 	if err != nil {
	// 		return err
	// 	}
	// }
}

func (c formURLEncoded[T]) unmarshalMap(query url.Values, n refvis.Node) error {
	var (
		ptr       = n.Value.Addr()
		mapType   = ptr.Type().Elem()
		keyType   = mapType.Key()
		valueType = mapType.Elem()
	)
	ptr.Elem().Set(reflect.MakeMap(mapType)) // create a new map[K]V

	// qKeyPrefix :=

	for qKey, qVS := range query {
		// out, err := slicekit.MapErr(qVS, func(i string) (any, error) {
		// 	return convkit.DuckParse(i)
		// })
		// if err != nil {
		// 	return fmt.Errorf("error while parsing %s's values: %w", qKey, err)
		// }

		mKey, err := convkit.ParseReflect(keyType, qKey)
		if err != nil {
			return fmt.Errorf("failed to parse key value for type %s: %w",
				keyType.String(), err)
		}

		mValP := reflect.New(valueType)
		pp.PP(mValP.Interface())
		mValP.Elem().Set(ptr.Elem().MapIndex(mKey))
		ptr.Elem().SetMapIndex(mKey, mValP.Elem())

		switch {
		case len(qVS) == 0:
			continue
		case 0 < len(qVS):
			switch valueType.Kind() {
			case reflect.Slice:
				mValP.Elem().Set(reflect.MakeSlice(valueType, len(qVS), len(qVS)))
				ptr.Elem().SetMapIndex(mKey, mValP.Elem())

			case reflect.Array:
				elemType := valueType.Elem()
				for i, raw := range qVS {
					err := convkit.UnmarshalReflect(elemType, []byte(raw), mValP.Elem().Index(i).Addr())
					if err != nil {
						return err
					}
				}
			default:
				raw, ok := slicekit.Last(qVS)
				if !ok {
					continue
				}
				err := convkit.UnmarshalReflect(valueType, []byte(raw), mValP)
				if err != nil {
					return err
				}
			}
		}
		ptr.Elem().SetMapIndex(mKey, mValP.Elem())
	}
	return nil
}

type formProperties struct {
	Name      string
	OmitEmpty bool
}

func (c formURLEncoded[T]) qKeyFor(k string) string {
	if len(c.prefix) != 0 {
		k = c.prefix + "." + k
	}
	return k
}

func (c formURLEncoded[T]) getFormProperties(typ reflect.StructField) formProperties {
	var prop formProperties
	prop.Name = c.fmtKey(typ.Name)
	c.lookupTag(typ, "url", &prop)
	c.lookupTag(typ, "form", &prop)
	return prop
}

func (c formURLEncoded[T]) lookupTag(typ reflect.StructField, tagKey string, prop *formProperties) {
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

func (c formURLEncoded[T]) NewListEncoder(w io.Writer) codec.StreamEncoderT[T] {
	return &formURLStreamEncoder[T]{formURLEncoded: c, Writer: w}
}

type formURLStreamEncoder[T any] struct {
	formURLEncoded[T]
	Writer io.Writer
	index  int
}

func (se *formURLStreamEncoder[T]) Encode(v T) error {
	enc := se.formURLEncoded.
		withPrefix(se.formURLEncoded.getCollection()).
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

func (c formURLEncoded[T]) NewListDecoder(r io.Reader) codec.StreamDecoderT[T] {
	return iterkit.FromPullIter(&formURLStreamDecoder[T]{formURLEncoded: c, Reader: r})
}

type formURLStreamDecoder[T any] struct {
	formURLEncoded[T]
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

	c.curSuffix = fmt.Sprintf("%s[%d]", c.formURLEncoded.getCollection(), c.index)
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

func (c *formURLStreamDecoder[T]) Value() codec.DecoderT[T] {
	return c
}

func (c *formURLStreamDecoder[T]) Decode(p *T) error {
	dec := c.formURLEncoded
	dec.prefix = c.curSuffix
	return dec.unmarshal(c.curQuery, reflect.ValueOf(p))
}

func (c *formURLStreamDecoder[T]) Close() error {
	if len(c.queryBuffer) != 0 {
		return fmt.Errorf("unprocessed query string are in the stream decoder:\n%s", c.queryBuffer.Encode())
	}
	return nil
}
