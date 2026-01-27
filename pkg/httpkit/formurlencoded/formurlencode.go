package formurlencoded

import (
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/httpkit/mediatype"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/frameless/port/codec"
)

type Codec struct {
	// Collection is the id/name of the collection type.
	// The default value is the short type name of T in snake case with an "s" suffix.
	// For example if T is
	// - `Item` -> `items`
	// - `User` -> `users`
	// - `UserEmail` -> `user_emails`
	// Collection string

	// StringCase is the formatter used to format the url keys.
	// Default: stringkit.ToSnake
	StringCase func(string) string
	prefix     string
}

func (c Codec) withPrefix(prefix string) Codec {
	if len(c.prefix) == 0 {
		c.prefix = prefix
		return c
	}
	c.prefix = c.prefix + "." + prefix
	return c
}

func (c *Codec) fmtKey(key string) string {
	if c.StringCase != nil {
		return c.StringCase(key)
	}
	return stringkit.ToSnake(key)
}

var pkgPrefix = regexp.MustCompile(`^[\w\d]+\.`)

func (c *Codec) typeName(typ reflect.Type) string {
	if name := typ.Name(); 0 < len(name) {
		return c.fmtKey(name)
	}
	raw := typ.String()
	raw = pkgPrefix.ReplaceAllString(raw, "")
	return c.fmtKey(raw)
}

var mediaTypeFormURLEncoded = map[string]struct{}{
	mediatype.FormUrlencoded: {},
}

func (c Codec) filterValues(values url.Values) url.Values {
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

func (c Codec) formatValues(values url.Values) {
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

func (c Codec) Marshal(v any) ([]byte, error) {
	var input = reflect.ValueOf(v)
	if input.Kind() == reflect.Slice {
		return nil, fmt.Errorf("[%w] Slice types is not supported by form url encoded format", codec.ErrNotSupported)
	}
	q := url.Values{}
	err := c.marshalAppend(q, "", input)
	return []byte(q.Encode()), err
}

func (c Codec) marshalAppend(vs url.Values, qKey string, val reflect.Value) error {
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

func (c Codec) Unmarshal(data []byte, ptr any) error {
	vs, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	if ptr == nil {
		return fmt.Errorf("nil pointer received")
	}
	p := reflect.ValueOf(ptr)
	if p.Kind() != reflect.Pointer {
		return fmt.Errorf("non pointer type received as ptr argument for Form URL Encoded Unmarshal")
	}
	if p.Elem().Kind() == reflect.Slice {
		return fmt.Errorf("[%w] Slice types is not supported by form url encoded format", codec.ErrNotSupported)
	}
	return c.visitUnmarshal(vs, p)
}

func (c Codec) getQueryKeyFor(node reftree.Node) (string, error) {
	var k []string
	for e := range node.Iter() {
		switch e.Type {
		case reftree.ArrayElem, reftree.SliceElem:
			k = append(k, strconv.Itoa(e.Index))
		case reftree.StructField:
			prop := c.getFormProperties(e.StructField)
			k = append(k, prop.Name)
		case reftree.MapValue:
			mKeyStr, err := convkit.FormatReflect(e.MapKey)
			if err != nil {
				return "", err
			}
			k = append(k, mKeyStr)
		}
	}
	return strings.Join(k, "."), nil
}

func (c Codec) getQueryFor(q url.Values, n reftree.Node) (url.Values, error) {
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

func (c Codec) getQueryKeyValue(query url.Values, node reftree.Node) ([]string, bool, error) {
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

func (c Codec) visitUnmarshal(q url.Values, val reflect.Value) error {
	return reftree.Walk(val, func(n reftree.Node) error {
		switch n.Type {
		case reftree.StructField:
			qVS, ok, err := c.getQueryKeyValue(q, n)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}

			raw, ok := slicekit.First(qVS)
			if !ok {
				return nil
			}

			var typ = n.Value.Type()
			if n.Type == reftree.StructField {
				typ = n.StructField.Type
			}

			ptr := reflect.New(n.StructField.Type)

			if err := convkit.UnmarshalReflect(typ, []byte(raw), ptr); err != nil {
				return err
			}

			n.Value.Set(ptr.Elem())
			return reftree.Skip

		case reftree.Map:
			sq, err := c.getQueryFor(q, n)
			if err != nil {
				return err
			}
			if len(sq) == 0 {
				return nil
			}
			n.Value.Set(reflect.MakeMap(n.Value.Type()))
			if err := c.unmarshalMap(q, n); err != nil {
				return err
			}
			return nil

		case reftree.Slice:
			qVS, ok, err := c.getQueryKeyValue(q, n)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			n.Value.Set(reflect.MakeSlice(n.Value.Type(), len(qVS), len(qVS)))
			fallthrough
		case reftree.Array:
			qVS, ok, err := c.getQueryKeyValue(q, n)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			var elemType = n.Value.Type().Elem()
			for i, raw := range qVS {
				if err := convkit.UnmarshalReflect(elemType, []byte(raw), n.Value.Index(i).Addr()); err != nil {
					return err
				}
			}
			return nil
		}
		return nil
	})
}

func (c Codec) unmarshalValue(q url.Values, n reftree.Node, ptr reflect.Value) error {
	qVS, ok, err := c.getQueryKeyValue(q, n)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	raw, ok := slicekit.First(qVS)
	if !ok {
		return nil
	}

	var typ = n.Value.Type()
	if n.Type == reftree.StructField {
		typ = n.StructField.Type
	}

	return convkit.UnmarshalReflect(typ, []byte(raw), ptr)
}

func (c Codec) unmarshalStruct(vs url.Values, ptr reflect.Value) error {
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

func (c Codec) unmarshalMap(query url.Values, n reftree.Node) error {
	var (
		ptr       = n.Value.Addr()
		mapType   = ptr.Type().Elem()
		keyType   = mapType.Key()
		valueType = mapType.Elem()
	)
	ptr.Elem().Set(reflect.MakeMap(mapType)) // create a new map[K]V

	// qKeyPrefix :=

	for qKey, qVS := range query {
		mKey, err := convkit.ParseReflect(keyType, qKey)
		if err != nil {
			return fmt.Errorf("failed to parse key value for type %s: %w",
				keyType.String(), err)
		}

		mValP := reflect.New(valueType)
		// mValP.Elem().Set(ptr.Elem().MapIndex(mKey))

		switch {
		case len(qVS) == 0:
			continue

		case 0 < len(qVS):
			switch valueType.Kind() {
			case reflect.Slice:
				mValP.Elem().Set(reflect.MakeSlice(valueType, len(qVS), len(qVS)))

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

func (c Codec) qKeyFor(k string) string {
	if len(c.prefix) != 0 {
		k = c.prefix + "." + k
	}
	return k
}

func (c Codec) getFormProperties(typ reflect.StructField) formProperties {
	var prop formProperties
	prop.Name = c.fmtKey(typ.Name)
	c.lookupTag(typ, "url", &prop)
	c.lookupTag(typ, "form", &prop)
	return prop
}

func (c Codec) lookupTag(typ reflect.StructField, tagKey string, prop *formProperties) {
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
