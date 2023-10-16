package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/stringcase"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/logger"
	"github.com/adamluzsi/testcase/random"
)

var defaultKeyFormatter = stringcase.ToSnake

func ExampleField() {
	logger.Error(context.Background(), "msg",
		logger.Field("key1", "value"),
		logger.Field("key2", "value"))
}

func ExampleRegisterFieldType() {
	type MyEntity struct {
		ID               string
		NonSensitiveData string
		SensitiveData    string
	}

	// at package level
	var _ = logger.RegisterFieldType(func(ent MyEntity) logger.LoggingDetail {
		return logger.Fields{
			"id":   ent.ID,
			"data": ent.NonSensitiveData,
		}
	})
}

func TestField(t *testing.T) {
	s := testcase.NewSpec(t)

	buf := testcase.Let(s, func(t *testcase.T) logger.StubOutput { return logger.Stub(t) }).
		EagerLoading(s)

	var (
		key   = let.UUID(s)
		value = testcase.Let[any](s, nil)
	)
	act := func(t *testcase.T) logger.LoggingDetail {
		return logger.Field(key.Get(t), value.Get(t))
	}

	afterLogging := func(t *testcase.T) {
		t.Helper()
		logger.Info(nil, "", act(t))
	}

	keyIsLogged := func(t *testcase.T) {
		t.Helper()
		t.Must.Contain(buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
	}

	s.When("value is int", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.Int()
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), strconv.Itoa(value.Get(t).(int)))
		})
	})

	s.When("value is string", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.StringNWithCharset(5, random.CharsetAlpha())
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", value.Get(t).(string)))
		})
	})

	s.When("value is a struct registered for logging", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logger.RegisterFieldType(func(ms MyStruct) logger.LoggingDetail {
			return logger.Fields{
				"foo": ms.Foo,
				"bar": ms.Bar,
				"baz": ms.Baz,
			}
		})

		myStruct := testcase.Let(s, func(t *testcase.T) MyStruct {
			return MyStruct{
				Foo: t.Random.UUID(),
				Bar: t.Random.UUID(),
				Baz: t.Random.UUID(),
			}
		})

		value.Let(s, func(t *testcase.T) any { return myStruct.Get(t) })

		s.Then("then the registered field mapping is used", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})

		s.And("the field value passed as a pointer", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return pointer.Of(myStruct.Get(t))
			})

			s.Then("then the registered field mapping is used", func(t *testcase.T) {
				afterLogging(t)
				keyIsLogged(t)

				t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
				t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
				t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
				t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			})
		})
	})

	s.When("value type is registered for logging as a pointer type", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logger.RegisterFieldType(func(ms *MyStruct) logger.LoggingDetail {
			return logger.Fields{
				"foo": ms.Foo,
				"bar": ms.Bar,
				"baz": ms.Baz,
			}
		})

		myStruct := testcase.Let(s, func(t *testcase.T) *MyStruct {
			return &MyStruct{
				Foo: t.Random.UUID(),
				Bar: t.Random.UUID(),
				Baz: t.Random.UUID(),
			}
		})

		value.Let(s, func(t *testcase.T) any { return myStruct.Get(t) })

		s.Then("then the registered field mapping is used", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})
	})

	s.When("value is a struct not registered for logging", func(s *testcase.Spec) {
		type MyUnregisteredStruct struct{ Foo string }

		value.Let(s, func(t *testcase.T) any {
			return MyUnregisteredStruct{Foo: t.Random.String()}
		})

		s.Then("field is ignored, but a warning is made", func(t *testcase.T) {
			afterLogging(t)

			t.Must.NotContain(buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
			t.Must.Contain(buf.Get(t).String(), "security concerns")
			t.Must.Contain(buf.Get(t).String(), "logger.RegisterFieldType")
		})
	})

	s.When("value is a map[string]T", func(s *testcase.Spec) {
		mapValue := let.UUID(s)
		value.Let(s, func(t *testcase.T) any {
			return map[string]string{
				"FooBar": mapValue.Get(t),
			}
		})

		s.Then("value is printed out", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			// snake is the default key formatting
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf(`"foo_bar":%q`, mapValue.Get(t)))
		})
	})
}

func ExampleFields() {
	logger.Error(context.Background(), "msg", logger.Fields{
		"key1": "value",
		"key2": "value",
	})
}

func TestFields(t *testing.T) {
	s := testcase.NewSpec(t)

	buf := testcase.Let(s, func(t *testcase.T) logger.StubOutput { return logger.Stub(t) }).
		EagerLoading(s)

	var (
		key   = let.UUID(s)
		value = testcase.Let[any](s, nil)
	)
	act := func(t *testcase.T) logger.LoggingDetail {
		return logger.Fields{key.Get(t): value.Get(t)}
	}

	afterLogging := func(t *testcase.T) {
		t.Helper()
		logger.Info(nil, "", act(t))
	}

	keyIsLogged := func(t *testcase.T) {
		t.Helper()
		t.Must.Contain(buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
	}

	s.When("value is int", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.Int()
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), strconv.Itoa(value.Get(t).(int)))
		})
	})

	s.When("value is string", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.StringNWithCharset(5, random.CharsetAlpha())
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", value.Get(t).(string)))
		})
	})

	s.When("value is a struct registered for logging", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logger.RegisterFieldType(func(ms MyStruct) logger.LoggingDetail {
			return logger.Fields{
				"foo": ms.Foo,
				"bar": ms.Bar,
				"baz": ms.Baz,
			}
		})

		myStruct := testcase.Let(s, func(t *testcase.T) MyStruct {
			return MyStruct{
				Foo: t.Random.UUID(),
				Bar: t.Random.UUID(),
				Baz: t.Random.UUID(),
			}
		})

		value.Let(s, func(t *testcase.T) any { return myStruct.Get(t) })

		s.Then("then the registered field mapping is used", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})
	})

	s.When("value is a struct not registered for logging", func(s *testcase.Spec) {
		type MyUnregisteredStruct struct{ Foo string }

		value.Let(s, func(t *testcase.T) any {
			return MyUnregisteredStruct{Foo: t.Random.String()}
		})

		s.Then("field is ignored, but a warning is made", func(t *testcase.T) {
			afterLogging(t)

			t.Must.NotContain(buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
			t.Must.Contain(buf.Get(t).String(), "security concerns")
			t.Must.Contain(buf.Get(t).String(), "logger.RegisterFieldType")
		})
	})

	s.When("value is a map[string]T", func(s *testcase.Spec) {
		mapValue := let.UUID(s)
		value.Let(s, func(t *testcase.T) any {
			return map[string]string{
				"FooBar": mapValue.Get(t),
			}
		})

		s.Then("value is printed out", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			// snake is the default key formatting
			t.Must.Contain(buf.Get(t).String(), fmt.Sprintf(`"foo_bar":%q`, mapValue.Get(t)))
		})
	})
}

func ExampleErrField() {
	ctx := context.Background()
	err := errors.New("boom")

	logger.Error(ctx, "task failed successfully", logger.ErrField(err))
}

func TestErrField(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("plain error", func(t *testing.T) {
		buf := logger.Stub(t)
		expErr := rnd.Error()
		logger.Info(nil, "boom", logger.ErrField(expErr))
		assert.Contain(t, buf.String(), `"error":{`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"message":%q`, expErr.Error()))
	})
	t.Run("nil error", func(t *testing.T) {
		buf := logger.Stub(t)
		logger.Info(nil, "boom", logger.ErrField(nil))
		assert.NotContain(t, buf.String(), `"error"`)
	})
	t.Run("when err is a user error", func(t *testing.T) {
		buf := logger.Stub(t)
		const message = "The answer"
		const code = "42"
		var expErr error
		expErr = errorkit.UserError{ID: code, Message: message}
		expErr = fmt.Errorf("err: %w", expErr)
		d := logger.ErrField(expErr)
		logger.Info(nil, "boom", d)
		assert.Contain(t, buf.String(), `"error":{`)
		assert.Contain(t, buf.String(), fmt.Sprintf(`"code":%q`, code))
		assert.Contain(t, buf.String(), fmt.Sprintf(`"message":%q`, expErr.Error()))
	})
}

type Foo struct {
	Bar Bar
}

var _ = logger.RegisterFieldType[Foo](func(foo Foo) logger.LoggingDetail {
	return logger.Field("bar", foo.Bar)
})

type Bar struct {
	V string
}

var _ = logger.RegisterFieldType[Bar](func(bar Bar) logger.LoggingDetail {
	return logger.Field("v", bar.V)
})

func TestField_nested(t *testing.T) {
	buf := logger.Stub(t)
	rnd := random.New(random.CryptoSeed{})
	val := rnd.String()
	foo := Foo{Bar: Bar{V: val}}
	logger.Info(nil, "message", logger.Field("foo", foo))

	type Out struct {
		Foo struct {
			Bar struct {
				V string `json:"v"`
			} `json:"bar"`
		} `json:"foo"`
		Level     string    `json:"level"`
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
	}

	var out Out
	assert.NoError(t, json.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&out)) // decode the first line
	assert.Equal(t, val, out.Foo.Bar.V)
}

func TestField_canNotOverrideBaseFields(t *testing.T) {
	buf := logger.Stub(t)
	rnd := random.New(random.CryptoSeed{})
	val := rnd.String()
	msg := rnd.String()
	logger.Info(nil, msg, logger.Field("message", val))
	type Out struct {
		Message string `json:"message"`
	}
	var out Out
	assert.NoError(t, json.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&out)) // decode the first line
	assert.Equal(t, msg, out.Message)
}
