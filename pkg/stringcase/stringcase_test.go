package stringcase_test

import (
	"github.com/adamluzsi/frameless/pkg/stringcase"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/pp"
	"github.com/adamluzsi/testcase/random"
	"strings"
	"testing"
)

func TestToSnake(t *testing.T) {
	type TC struct {
		In  string
		Out string
	}
	testcase.TableTest(t, map[string]TC{
		"empty string":                    {In: "", Out: ""},
		"one character":                   {In: "A", Out: "a"},
		"snake":                           {In: "hello_world", Out: "hello_world"},
		"pascal":                          {In: "HelloWorld", Out: "hello_world"},
		"pascal with multiple words":      {In: "HTTPFoo", Out: "http_foo"},
		"camel":                           {In: "helloWorld", Out: "hello_world"},
		"upper":                           {In: "HELLO WORLD", Out: "hello_world"},
		"screaming snake":                 {In: "HELLO_WORLD", Out: "hello_world"},
		"dot case":                        {In: "hello.world", Out: "hello_world"},
		"kebab case":                      {In: "hello-world", Out: "hello_world"},
		"handles utf-8 characters":        {In: "Héllo Wörld", Out: "héllo_wörld"},
		"mixture of Title and lower case": {In: "the Hello World", Out: "the_hello_world"},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.ToSnake(tc.In), "original:", assert.Message(pp.Format(tc.In)))
	})
}

func BenchmarkToSnake(b *testing.B) {
	rnd := random.New(random.CryptoSeed{})
	fixtures := random.Slice(b.N, func() string {
		return rnd.StringNC(10,
			strings.ToLower(random.CharsetAlpha())+
				strings.ToLower(random.CharsetAlpha())+
				"_.-")
	})
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stringcase.ToSnake(fixtures[i])
	}
}

func TestIsSnake(t *testing.T) {
	type TC struct {
		In  string
		Out bool
	}
	testcase.TableTest(t, map[string]TC{
		"snake case":                                  {In: "hello_world", Out: true},
		"snake case with utf-8 characters":            {In: "héllo_wörld", Out: true},
		"pascal case":                                 {In: "HelloWorld", Out: false},
		"pascal case with abbrevation":                {In: "HTTPFoo", Out: false},
		"mixed case with number suffix + pascal case": {In: "HelloWorld42", Out: false},
		"pascal case with utf-8 characters":           {In: "HélloWörld", Out: false},
		"camel case":                                  {In: "helloWorld", Out: false},
		"title snake case":                            {In: "Hello_World", Out: false},
		"title dot case":                              {In: "Hello.World", Out: false},
		"title kebab case":                            {In: "Hello-World", Out: false},
		"mixed case with number prefix + pascal case": {In: "1HelloWorld", Out: false},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.IsSnake(tc.In), "input:", assert.Message(pp.Format(tc.In)))
	})
}

func TestIsScreamingSnake(t *testing.T) {
	type TC struct {
		In  string
		Out bool
	}
	testcase.TableTest(t, map[string]TC{
		"snake case":                                  {In: "HELLO_WORLD", Out: true},
		"snake case with utf-8 characters":            {In: "HÉLLO_WÖRLD", Out: true},
		"pascal case":                                 {In: "HelloWorld", Out: false},
		"pascal case with abbrevation":                {In: "HTTPFoo", Out: false},
		"mixed case with number suffix + pascal case": {In: "HelloWorld42", Out: false},
		"pascal case with utf-8 characters":           {In: "HélloWörld", Out: false},
		"camel case":                                  {In: "helloWorld", Out: false},
		"title snake case":                            {In: "Hello_World", Out: false},
		"title dot case":                              {In: "Hello.World", Out: false},
		"title kebab case":                            {In: "Hello-World", Out: false},
		"mixed case with number prefix + pascal case": {In: "1HelloWorld", Out: false},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.IsScreamingSnake(tc.In), "input:", assert.Message(pp.Format(tc.In)))
	})
}

func TestToScreamingSnake(t *testing.T) {
	type TC struct {
		In  string
		Out string
	}
	testcase.TableTest(t, map[string]TC{
		"empty string":                    {In: "", Out: ""},
		"one character":                   {In: "a", Out: "A"},
		"snake":                           {In: "hello_world", Out: "HELLO_WORLD"},
		"pascal":                          {In: "HelloWorld", Out: "HELLO_WORLD"},
		"pascal with multiple words":      {In: "HTTPFoo", Out: "HTTP_FOO"},
		"camel":                           {In: "helloWorld", Out: "HELLO_WORLD"},
		"upper":                           {In: "HELLO WORLD", Out: "HELLO_WORLD"},
		"screaming snake":                 {In: "HELLO_WORLD", Out: "HELLO_WORLD"},
		"dot case":                        {In: "hello.world", Out: "HELLO_WORLD"},
		"kebab case":                      {In: "hello-world", Out: "HELLO_WORLD"},
		"handles utf-8 characters":        {In: "Héllo Wörld", Out: "HÉLLO_WÖRLD"},
		"mixture of Title and lower case": {In: "the Hello World", Out: "THE_HELLO_WORLD"},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.ToScreamingSnake(tc.In), "original:", assert.Message(pp.Format(tc.In)))
	})
}

func TestIsPascal(t *testing.T) {
	type TC struct {
		In  string
		Out bool
	}
	testcase.TableTest(t, map[string]TC{
		"pascal case":                                 {In: "HelloWorld", Out: true},
		"pascal case with abbrevation":                {In: "HTTPFoo", Out: true},
		"mixed case with number suffix + pascal case": {In: "HelloWorld42", Out: true},
		"pascal case with utf-8 characters":           {In: "HélloWörld", Out: true},
		"camel case":                                  {In: "helloWorld", Out: false},
		"title snake case":                            {In: "Hello_World", Out: false},
		"title dot case":                              {In: "Hello.World", Out: false},
		"title kebab case":                            {In: "Hello-World", Out: false},
		"mixed case with number prefix + pascal case": {In: "1HelloWorld", Out: false},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.IsPascal(tc.In), "input:", assert.Message(pp.Format(tc.In)))
	})
}

func TestToPascal(t *testing.T) {
	type TC struct {
		In  string
		Out string
	}
	testcase.TableTest(t, map[string]TC{
		"empty string":                    {In: "", Out: ""},
		"one upper character":             {In: "A", Out: "A"},
		"one lower character":             {In: "a", Out: "A"},
		"snake":                           {In: "hello_world", Out: "HelloWorld"},
		"pascal":                          {In: "HelloWorld", Out: "HelloWorld"},
		"pascal with multiple words":      {In: "HTTPFoo", Out: "HTTPFoo"},
		"camel":                           {In: "helloWorld", Out: "HelloWorld"},
		"screaming snake":                 {In: "HELLO_WORLD", Out: "HelloWorld"},
		"dot case":                        {In: "hello.world", Out: "HelloWorld"},
		"kebab case":                      {In: "hello-world", Out: "HelloWorld"},
		"handles utf-8 characters":        {In: "Héllo Wörld", Out: "HélloWörld"},
		"mixture of Title and lower case": {In: "the Hello World", Out: "TheHelloWorld"},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.ToPascal(tc.In), "original:", assert.Message(pp.Format(tc.In)))
	})
}

func TestIsCamel(t *testing.T) {
	type TC struct {
		In  string
		Out bool
	}
	testcase.TableTest(t, map[string]TC{
		"camel case":                                  {In: "helloWorld", Out: true},
		"camel case with utf-8 characters":            {In: "hélloWörld", Out: true},
		"pascal case":                                 {In: "HelloWorld", Out: false},
		"pascal case with abbrevation":                {In: "HTTPFoo", Out: false},
		"mixed case with number suffix + pascal case": {In: "HelloWorld42", Out: false},
		"pascal case with utf-8 characters":           {In: "HélloWörld", Out: false},
		"title snake case":                            {In: "Hello_World", Out: false},
		"title dot case":                              {In: "Hello.World", Out: false},
		"title kebab case":                            {In: "Hello-World", Out: false},
		"mixed case with number prefix + pascal case": {In: "1HelloWorld", Out: false},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.IsCamel(tc.In), "input:", assert.Message(pp.Format(tc.In)))
	})
}

func TestToCamel(t *testing.T) {
	type TC struct {
		In  string
		Out string
	}
	testcase.TableTest(t, map[string]TC{
		"empty string":                    {In: "", Out: ""},
		"one upper character":             {In: "A", Out: "a"},
		"one lower character":             {In: "a", Out: "a"},
		"snake":                           {In: "hello_world", Out: "helloWorld"},
		"pascal":                          {In: "HelloWorld", Out: "helloWorld"},
		"pascal with multiple words v1":   {In: "HTTPFoo", Out: "httpFoo"},
		"pascal with multiple words v2":   {In: "VFoo", Out: "vFoo"},
		"camel":                           {In: "helloWorld", Out: "helloWorld"},
		"screaming snake":                 {In: "HELLO_WORLD", Out: "helloWorld"},
		"dot case":                        {In: "hello.world", Out: "helloWorld"},
		"kebab case":                      {In: "hello-world", Out: "helloWorld"},
		"handles utf-8 characters":        {In: "Héllo Wörld", Out: "hélloWörld"},
		"mixture of Title and lower case": {In: "the Hello World", Out: "theHelloWorld"},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.ToCamel(tc.In), "original:", assert.Message(pp.Format(tc.In)))
	})
}

func BenchmarkToPascal(b *testing.B) {
	rnd := random.New(random.CryptoSeed{})
	fixtures := random.Slice(b.N, func() string {
		return rnd.StringNC(10,
			strings.ToLower(random.CharsetAlpha())+
				strings.ToLower(random.CharsetAlpha())+
				"_.-")
	})
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stringcase.ToPascal(fixtures[i])
	}
}

func TestIsKebab(t *testing.T) {
	type TC struct {
		In  string
		Out bool
	}
	testcase.TableTest(t, map[string]TC{
		"snake case":                                  {In: "hello_world", Out: true},
		"snake case with utf-8 characters":            {In: "héllo_wörld", Out: true},
		"pascal case":                                 {In: "HelloWorld", Out: false},
		"pascal case with abbrevation":                {In: "HTTPFoo", Out: false},
		"mixed case with number suffix + pascal case": {In: "HelloWorld42", Out: false},
		"pascal case with utf-8 characters":           {In: "HélloWörld", Out: false},
		"camel case":                                  {In: "helloWorld", Out: false},
		"title snake case":                            {In: "Hello_World", Out: false},
		"title dot case":                              {In: "Hello.World", Out: false},
		"title kebab case":                            {In: "Hello-World", Out: false},
		"mixed case with number prefix + pascal case": {In: "1HelloWorld", Out: false},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.IsSnake(tc.In), "input:", assert.Message(pp.Format(tc.In)))
	})
}

func TestToKebab(t *testing.T) {
	type TC struct {
		In  string
		Out string
	}
	testcase.TableTest(t, map[string]TC{
		"empty string":                    {In: "", Out: ""},
		"one character":                   {In: "A", Out: "a"},
		"snake":                           {In: "hello_world", Out: "hello-world"},
		"pascal":                          {In: "HelloWorld", Out: "hello-world"},
		"pascal with multiple words":      {In: "HTTPFoo", Out: "http-foo"},
		"camel":                           {In: "helloWorld", Out: "hello-world"},
		"upper":                           {In: "HELLO WORLD", Out: "hello-world"},
		"screaming snake":                 {In: "HELLO_WORLD", Out: "hello-world"},
		"dot case":                        {In: "hello.world", Out: "hello-world"},
		"kebab case":                      {In: "hello-world", Out: "hello-world"},
		"handles utf-8 characters":        {In: "Héllo Wörld", Out: "héllo-wörld"},
		"mixture of Title and lower case": {In: "the Hello World", Out: "the-hello-world"},
	}, func(t *testcase.T, tc TC) {
		t.Must.Equal(tc.Out, stringcase.ToKebab(tc.In), "original:", assert.Message(pp.Format(tc.In)))
	})
}
