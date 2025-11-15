package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/stringkit"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/testcase/random"
)

var defaultKeyFormatter = stringkit.ToSnake

func ExampleField() {
	var l logging.Logger

	l.Error(context.Background(), "msg",
		logging.Field("key1", "value"),
		logging.Field("key2", "value"))
}

func ExampleRegisterType_asLoggingDetails() {
	type MyEntity struct {
		ID               string
		NonSensitiveData string
		SensitiveData    string
	}

	// at package level
	var _ = logging.RegisterType(func(ctx context.Context, ent MyEntity) logging.Detail {
		return logging.Fields{
			"id":   ent.ID,
			"data": ent.NonSensitiveData,
		}
	})
}

func TestField(t *testing.T) {
	s := testcase.NewSpec(t)
	logger, buf := testcase.Let2(s, func(t *testcase.T) (*logging.Logger, logging.StubOutput) {
		return logging.Stub(t)
	})

	var (
		key   = let.UUID(s)
		value = testcase.Let[any](s, nil)
	)
	act := func(t *testcase.T) logging.Detail {
		return logging.Field(key.Get(t), value.Get(t))
	}

	afterLogging := func(t *testcase.T) {
		t.Helper()
		logger.Get(t).Info(nil, "", act(t))
	}

	keyIsLogged := func(t *testcase.T) {
		t.Helper()
		assert.Contains(t, buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
	}

	s.When("value is int", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.Int()
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), strconv.Itoa(value.Get(t).(int)))
		})

		s.And("is a sub type", func(s *testcase.Spec) {
			type IntType int
			value.Let(s, func(t *testcase.T) any {
				return IntType(t.Random.Int())
			})

			s.Then("it automatically take it as a string", func(t *testcase.T) {
				afterLogging(t)
				keyIsLogged(t)

				assert.Contains(t, buf.Get(t).String(), strconv.Itoa(int(value.Get(t).(IntType))))
			})
		})
	})

	s.When("value is string", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.StringNWithCharset(5, random.CharsetAlpha())
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", value.Get(t).(string)))
		})

		s.And("is a sub type", func(s *testcase.Spec) {
			type StringType string
			value.Let(s, func(t *testcase.T) any {
				return StringType(t.Random.String())
			})

			s.Then("it automatically take it as a string", func(t *testcase.T) {
				afterLogging(t)
				keyIsLogged(t)

				data, err := json.Marshal(value.Get(t).(StringType))
				assert.NoError(t, err)

				assert.Contains(t, buf.Get(t).String(), string(data))
			})
		})
	})

	s.When("value is a struct registered for logging", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logging.RegisterType(func(ctx context.Context, ms MyStruct) logging.Detail {
			return logging.Fields{
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

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})

		s.And("the field value passed as a pointer", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return pointer.Of(myStruct.Get(t))
			})

			s.Then("then the registered field mapping is used", func(t *testcase.T) {
				afterLogging(t)
				keyIsLogged(t)

				assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
				assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
				assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
				assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			})
		})
	})

	s.When("value type is registered for logging as a pointer type", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logging.RegisterType(func(ctx context.Context, ms *MyStruct) logging.Detail {
			return logging.Fields{
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

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})
	})

	s.When("value is a struct not registered for logging", func(s *testcase.Spec) {
		type MyUnregisteredStruct struct{ Foo string }

		value.Let(s, func(t *testcase.T) any {
			return MyUnregisteredStruct{Foo: t.Random.String()}
		})

		s.Then("field is ignored, but a warning is made", func(t *testcase.T) {
			afterLogging(t)

			assert.NotContains(t, buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
			assert.Contains(t, buf.Get(t).String(), "security concerns")
			assert.Contains(t, buf.Get(t).String(), "logger.RegisterType")
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

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			// snake is the default key formatting
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf(`"foo_bar":%q`, mapValue.Get(t)))
		})
	})

	s.When("value is implementing an interface type which is registered for logging", func(s *testcase.Spec) {
		logging.RegisterType(func(ctx context.Context, mi MyInterface) logging.Detail {
			return logging.Field("IDDQD", mi.GetIDDQD())
		})

		myData := testcase.Let(s, func(t *testcase.T) MyData {
			return MyData{ID: t.Random.UUID()}
		})

		value.Let(s, func(t *testcase.T) any { return myData.Get(t) })

		s.Then("then the registered field mapping is used", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:%q", defaultKeyFormatter("IDDQD"), myData.Get(t).ID))
		})
	})

	s.When("value is pointer type", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return pointer.Of(t.Random.StringNWithCharset(5, random.CharsetAlpha()))
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", *value.Get(t).(*string)))
		})

		s.And("it is a constructed nil pointer", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				var ptr *string
				return ptr
			})

			s.Then("field is logged as nil/null", func(t *testcase.T) {
				afterLogging(t)
				keyIsLogged(t)

				assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:null", defaultKeyFormatter(key.Get(t))))
			})
		})
	})

	s.When("value is nil", func(s *testcase.Spec) {
		value.LetValue(s, nil)

		s.Then("field is logged as nil/null", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:null", defaultKeyFormatter(key.Get(t))))
		})
	})
}

func ExampleLazyDetail() {
	var l logging.Logger

	l.Debug(context.Background(), "msg", logging.LazyDetail(func() logging.Detail {
		// only runs if the logging level is enabled and the logging details are collected
		return logging.Field("key1", "value")
	}))
}

func TestLazyDetail(t *testing.T) {
	ctx := context.Background()
	t.Run("happy-path", func(t *testing.T) {
		l, buf := logging.Stub(t)
		l.Level = logging.LevelInfo

		var called bool
		fn := logging.LazyDetail(func() logging.Detail {
			called = true
			return logging.Field("foo", "bar")
		})

		l.Debug(ctx, "", fn)
		assert.False(t, called)

		l.Info(ctx, "", fn)
		assert.True(t, called)
		assert.Contains(t, buf.String(), "foo")
		assert.Contains(t, buf.String(), "bar")
	})

	t.Run("on nil detail result", func(t *testing.T) {
		l, buf := logging.Stub(t)
		l.Level = logging.LevelInfo

		var called bool
		fn := logging.LazyDetail(func() logging.Detail {
			called = true
			return nil
		})

		l.Debug(ctx, "", fn)
		assert.False(t, called)

		assert.NotPanic(t, func() {
			l.Info(ctx, "msg", fn)
		})
		assert.True(t, called)
		assert.Contains(t, buf.String(), "msg")
	})

	t.Run("on nil func", func(t *testing.T) {
		l, buf := logging.Stub(t)
		l.Level = logging.LevelInfo

		fn := logging.LazyDetail(nil)

		assert.NotPanic(t, func() {
			l.Debug(ctx, "", fn)
			l.Info(ctx, "msg", fn)
		})
		assert.Contains(t, buf.String(), "msg")
	})
}

func TestRegisterType_unregisterTypeCallback(t *testing.T) {
	t.Run("for concrete type", func(t *testing.T) {
		type X struct{ V int }
		l, buf := logging.Stub(t)
		unregister := logging.RegisterType[X](func(ctx context.Context, x X) logging.Detail {
			return logging.Fields{"v": x.V}
		})

		l.Info(nil, "msg", logging.Field("x", X{V: 123456789}))
		assert.Contains(t, buf.String(), "123456789")

		unregister()
		l.Info(nil, "msg", logging.Field("x", X{V: 987654321}))
		assert.NotContains(t, buf.String(), "987654321")
		assert.Contains(t, buf.String(), "security")
	})
	t.Run("for interface", func(t *testing.T) {
		l, buf := logging.Stub(t)
		unregister := logging.RegisterType[testent.Fooer](func(ctx context.Context, fooer testent.Fooer) logging.Detail {
			return logging.Fields{"foo": fooer.GetFoo()}
		})

		l.Info(nil, "msg", logging.Field("x", testent.Foo{Foo: "123456789"}))
		assert.Contains(t, buf.String(), "123456789")

		unregister()
		l.Info(nil, "msg", logging.Field("x", testent.Foo{Foo: "987654321"}))
		assert.NotContains(t, buf.String(), "987654321")
		assert.Contains(t, buf.String(), "security")
	})
}

func ExampleFields() {
	var l logging.Logger
	l.Error(context.Background(), "msg", logging.Fields{
		"key1": "value",
		"key2": "value",
	})
}

func TestFields(t *testing.T) {
	s := testcase.NewSpec(t)

	logger, buf := testcase.Let2(s, func(t *testcase.T) (*logging.Logger, logging.StubOutput) {
		return logging.Stub(t)
	})

	var (
		key   = let.UUID(s)
		value = testcase.Let[any](s, nil)
	)
	act := func(t *testcase.T) logging.Detail {
		return logging.Fields{key.Get(t): value.Get(t)}
	}

	afterLogging := func(t *testcase.T) {
		t.Helper()
		logger.Get(t).Info(nil, "", act(t))
	}

	keyIsLogged := func(t *testcase.T) {
		t.Helper()
		assert.Contains(t, buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
	}

	s.When("value is int", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.Int()
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), strconv.Itoa(value.Get(t).(int)))
		})
	})

	s.When("value is string", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return t.Random.StringNWithCharset(5, random.CharsetAlpha())
		})

		s.Then("field is logged", func(t *testcase.T) {
			afterLogging(t)
			keyIsLogged(t)

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", value.Get(t).(string)))
		})
	})

	s.When("value is a struct registered for logging", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logging.RegisterType(func(ctx context.Context, ms MyStruct) logging.Detail {
			return logging.Fields{
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

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})
	})

	s.When("value is a struct not registered for logging", func(s *testcase.Spec) {
		type MyUnregisteredStruct struct{ Foo string }

		value.Let(s, func(t *testcase.T) any {
			return MyUnregisteredStruct{Foo: t.Random.String()}
		})

		s.Then("field is ignored, but a warning is made", func(t *testcase.T) {
			afterLogging(t)

			assert.NotContains(t, buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
			assert.NotContains(t, buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
			assert.NotContains(t, buf.Get(t).String(), fmt.Sprintf(`%q:`, defaultKeyFormatter(key.Get(t))))
			assert.Contains(t, buf.Get(t).String(), "security concerns")
			assert.Contains(t, buf.Get(t).String(), "logger.RegisterType")
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

			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf("%q:{", defaultKeyFormatter(key.Get(t))))
			// snake is the default key formatting
			assert.Contains(t, buf.Get(t).String(), fmt.Sprintf(`"foo_bar":%q`, mapValue.Get(t)))
		})
	})
}

func ExampleErrField() {
	ctx := context.Background()
	err := errors.New("boom")
	var l logging.Logger

	l.Error(ctx, "task failed successfully", logging.ErrField(err))
}

func TestErrField(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("plain error", func(t *testing.T) {
		l, buf := logging.Stub(t)
		expErr := rnd.Error()
		l.Info(nil, "boom", logging.ErrField(expErr))
		assert.Contains(t, buf.String(), `"error":{`)
		assert.Contains(t, buf.String(), fmt.Sprintf(`"message":%q`, expErr.Error()))
	})
	t.Run("nil error", func(t *testing.T) {
		l, buf := logging.Stub(t)
		l.Info(nil, "boom", logging.ErrField(nil))
		assert.NotContains(t, buf.String(), `"error"`)
	})
	t.Run("when err is a user error", func(t *testing.T) {
		l, buf := logging.Stub(t)
		const message = "The answer"
		const code = "42"
		var expErr error
		expErr = errorkit.UserError{Code: code, Message: message}
		expErr = fmt.Errorf("err: %w", expErr)
		d := logging.ErrField(expErr)
		l.Info(nil, "boom", d)
		assert.Contains(t, buf.String(), `"error":{`)
		assert.Contains(t, buf.String(), fmt.Sprintf(`"code":%q`, code))
		assert.Contains(t, buf.String(), fmt.Sprintf(`"message":%q`, expErr.Error()))
	})
	t.Run("when error handling registered", func(t *testing.T) {
		t.Cleanup(logging.RegisterType[error](func(ctx context.Context, err error) logging.Detail {
			return logging.Field("err", err.Error())
		}))

		l, buf := logging.Stub(t)
		var expErr error = rnd.Error()
		d := logging.ErrField(expErr)
		l.Info(t.Context(), "boom", d)
		assert.Contains(t, buf.String(), fmt.Sprintf(`"err":%q`, expErr.Error()))
	})
}

type Foo struct {
	Bar Bar
}

var _ = logging.RegisterType[Foo](func(ctx context.Context, foo Foo) logging.Detail {
	return logging.Field("bar", foo.Bar)
})

type Bar struct {
	V string
}

var _ = logging.RegisterType[Bar](func(ctx context.Context, bar Bar) logging.Detail {
	return logging.Field("v", bar.V)
})

func TestField_nested(t *testing.T) {
	l, buf := logging.Stub(t)
	rnd := random.New(random.CryptoSeed{})
	val := rnd.String()
	foo := Foo{Bar: Bar{V: val}}
	l.Info(nil, "message", logging.Field("foo", foo))

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
	l, buf := logging.Stub(t)
	rnd := random.New(random.CryptoSeed{})
	val := rnd.String()
	msg := rnd.String()
	l.Info(context.Background(), msg, logging.Field("message", val))
	type Out struct {
		Message string `json:"message"`
	}
	var out Out
	assert.NoError(t, json.NewDecoder(bytes.NewReader(buf.Bytes())).Decode(&out)) // decode the first line
	assert.Equal(t, msg, out.Message)
}

type MyInterface interface{ GetIDDQD() string }

type MyData struct{ ID string }

func (d MyData) GetIDDQD() string { return d.ID }

func TestWith(t *testing.T) {
	s := testcase.NewSpec(t)

	logger, logs := testcase.Let2(s, func(t *testcase.T) (*logging.Logger, logging.StubOutput) {
		return logging.Stub(t)
	})

	var value = testcase.Let[any](s, nil)
	act := func(t *testcase.T) logging.Detail {
		return logging.With(value.Get(t))
	}

	afterLogging := func(t *testcase.T) {
		t.Helper()
		logger.Get(t).Info(t.Context(), "", act(t))
	}

	s.When("value is an unregistered logging detail type", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) any {
			return random.Pick(t.Random,
				func() any { return t.Random.StringNWithCharset(5, random.CharsetAlpha()) },
				func() any { return t.Random.Int() },
				func() any { return t.Random.Bool() },
			)()
		})

		s.Then("warn issued", func(t *testcase.T) {
			afterLogging(t)
			assert.Contains(t, logs.Get(t).String(), `"level":"warn"`)
		})
	})

	s.When("value is a struct registered for logging", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		s.Before(func(t *testcase.T) {
			unregister := logging.RegisterType(func(ctx context.Context, ms MyStruct) logging.Detail {
				return logging.Fields{
					"foo": ms.Foo,
					"bar": ms.Bar,
					"baz": ms.Baz,
				}
			})
			t.Cleanup(unregister)
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

			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})

		s.And("the field value passed as a pointer", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return pointer.Of(myStruct.Get(t))
			})

			s.Then("then the registered field mapping is used", func(t *testcase.T) {
				afterLogging(t)

				assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
				assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
				assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			})
		})
	})

	s.When("value type is registered for logging as a pointer type", func(s *testcase.Spec) {
		type MyStruct struct {
			Foo string
			Bar string
			Baz string
		}

		logging.RegisterType(func(ctx context.Context, ms *MyStruct) logging.Detail {
			return logging.Fields{
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

			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Foo))
			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q", myStruct.Get(t).Baz))
		})
	})

	s.When("value is a struct not registered for logging", func(s *testcase.Spec) {
		type MyUnregisteredStruct struct{ Foo string }

		value.Let(s, func(t *testcase.T) any {
			return MyUnregisteredStruct{Foo: t.Random.String()}
		})

		s.Then("field is ignored, but a warning is made", func(t *testcase.T) {
			afterLogging(t)

			assert.Contains(t, logs.Get(t).String(), "security concerns")
			assert.Contains(t, logs.Get(t).String(), "logger.RegisterType")
		})
	})

	s.When("value is implementing an interface type which is registered for logging", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			unregister := logging.RegisterType(func(ctx context.Context, mi MyInterface) logging.Detail {
				return logging.Field("IDDQD", mi.GetIDDQD())
			})
			t.Cleanup(unregister)
		})

		myData := testcase.Let(s, func(t *testcase.T) MyData {
			return MyData{ID: t.Random.UUID()}
		})

		value.Let(s, func(t *testcase.T) any { return myData.Get(t) })

		s.Then("then the registered field mapping is used", func(t *testcase.T) {
			afterLogging(t)

			assert.Contains(t, logs.Get(t).String(), fmt.Sprintf("%q:%q", defaultKeyFormatter("IDDQD"), myData.Get(t).ID))
		})
	})

	s.When("value is nil", func(s *testcase.Spec) {
		value.LetValue(s, nil)

		s.Then("logging detail is ignored logged as nil/null", func(t *testcase.T) {
			afterLogging(t)

			out := logs.Get(t).String()
			dec := json.NewDecoder(strings.NewReader(out))

			for dec.More() {
				var record map[string]any
				assert.NoError(t, dec.Decode(&record))
				assert.ContainsExactly(t, mapkit.Keys(record),
					[]string{defaultLevelKey, defaultMessageKey, defaultTimestampKey},
					assert.MessageF("logs: %s", out))
			}

		})
	})
}
