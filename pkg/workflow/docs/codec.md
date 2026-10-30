# Codec

> Goal: understand why a workflow needs a `Codec` at all, use the built-in JSON
> one, and teach it about your own types.

**One sentence:** `Definition`, `Condition`, and `Event` are _interfaces_, so
saving one means the wire format has to remember **which concrete type it was** —
that is the entire job of a `Codec`.

---

## 1. The problem

`encoding/json` can serialise a definition just fine. It cannot bring it back:

```go
var def workflow.Definition = workflow.SetVar{Name: "name", Value: "World"}

data, _ := json.Marshal(def)      // {"Name":"name","Value":"World"}

var got workflow.Definition
json.Unmarshal(data, &got)
// json: cannot unmarshal object into Go value of type workflow.Definition
```

The bytes say _what the values were_, not _what type held them_. Since
[Definitions][DEFINITION] are the thing you store in your database and ship
between your front-end and your back-end, that is fatal.

A `Codec` fixes it by tagging the concrete type on the way out.

---

## 2. The built-in solution

```go
import "go.llib.dev/frameless/pkg/workflow/wfjson"

c := wfjson.NewCodec()

var def workflow.Definition = workflow.Sequence{
	workflow.SetVar{Name: "name", Value: "World"},
	workflow.Execute{Participant: "greet", Input: []workflow.VarName{"name"}},
}

data, err := c.Marshal(jsonkit.Interface[workflow.Definition]{V: def})

var got workflow.Definition
err = c.Unmarshal(data, &got) // a workflow.Sequence again, not a map[string]any
```

`Marshal` is given the value held by a `jsonkit.Interface[workflow.Definition]`,
which says explicitly that an interface holds the value, so it writes the `$type`.
Given the value itself, like `c.Marshal(def)`, it writes the value the way it writes any concrete type,
without the `$type`, as the caller is expected to read it back into the same concrete type.

A pointer to the interface, `c.Marshal(&def)`, writes the very same JSON, the same way as `Unmarshal` is given `&got`.
Prefer it where the code has to work with any `workflow.Codec`, not only with `jsonkit`,
like a repository or the contract tests: `encoding/json` writes `&def` the same as `def`,
while only a `jsonkit.Codec` knows the types registered on it through a `jsonkit.Interface`.

`wfjson.NewCodec()` returns a **fresh** `*jsonkit.Codec` on every call. Each one
owns a private registry of type tags, so two codecs never leak registrations into
each other — which is also why the contract suite can hand every scenario its own
instance.

---

## 3. What the wire looks like

`jsonkit` wraps each polymorphic value in a **type-tagged envelope** keyed on
`$type`:

```json
{
  "$type": "workflow:sequence",
  "$value": [
    { "$type": "workflow:var:set", "name": "name", "value": "World" },
    { "$type": "workflow:execute", "participant": "greet", "input": ["name"] }
  ]
}
```

Three envelope shapes exist, picked automatically:

| Encoded value is…                 | Shape                               | Example                                     |
| --------------------------------- | ----------------------------------- | ------------------------------------------- |
| a JSON object, other than a map's | `$type` inlined as an extra field   | `SetVar`, `If`                              |
| a map                             | `{"$type": …, "$map": {…}}` wrapper | a registered map type, a `map[string]int`   |
| anything else                     | `{"$type": …, "$value": …}` wrapper | `Sequence` (an array)                       |

A map's entries are the `$map` member, as they may have any key, like a `$type`:
there they are just data.

`$type`, `$value` and `$map` are `jsonkit`'s own members, and the format is **not
JSON-LD**: `$type` is not JSON-LD's `@type`, and a type ID like
`workflow:sequence` is a namespaced name, `prefix:name`, not an IRI. The
`workflow:` prefix only keeps the built-in type IDs apart from yours, so name
yours with a prefix of your own, like `acme:charge-order`.

Event logs and definitions written by earlier versions spell the type IDs
with a double colon, like `workflow::sequence`. They are still read the same way.
The members are matched by their exact names, so an `@type`, or an `@value`, is
just a member of the value. That way a type can have a `json:"@type"` field of
its own, like for a JSON-LD document.

The field names inside the envelope (`name`, `id`, `input`, …) come from private
DTO structs in `wfjson`, **not** from struct tags on the `workflow` types. That
is deliberate: the `workflow` package stays free of any JSON knowledge, and the
wire format can evolve without touching the domain types.

### Values held as `any`

Variable values (`SetVar.Value`, `EventSetVar.Value`) and the recorded
arguments of participants and conditions (`EventParticipant.Input`/`Output`,
`EventCondition.Input`) are `any`. `wfjson` writes them like this:

| The value is…                                                                       | Written as                                    | Read back as                              |
| ----------------------------------------------------------------------------------- | --------------------------------------------- | ----------------------------------------- |
| `nil`, `bool`, `string`, or `[]any` / `map[string]any` of values                   | plain JSON, as `encoding/json` writes it      | the same value                            |
| an `int`                                                                            | a plain JSON number, like `42`                | an `int`                                  |
| a `float64`                                                                         | a plain JSON number, with a `.0` fraction when it is a whole number, like `42.0` or `1.5` | a `float64`  |
| a number of another built-in type, like an `int64`, `uint8` or `float32`           | `{"$type":"int64","$value":42}`               | the same type                             |
| of a type registered with the codec                                                 | its `$type` envelope                          | the registered type                       |
| a slice or `string`-keyed map of a registered type, a workflow interface, or a built-in type, like `[]workflow.Definition` or `[]string` | `{"$type":"[]workflow:definition","$value":[…]}`, or `{"$type":"map[string]int","$map":{…}}` | the same slice or map type |
| anything else, like a `time.Time` or a struct of a type that isn't registered      | plain JSON, as `encoding/json` writes it; the values in its interface fields keep their `$type` | as `encoding/json` reads it, like a `string` or a `map[string]any` |

A number keeps its type, so a template condition like `lt .i 2` compares an
`int` with an `int`, whether the history is held in memory or as JSON.
A plain JSON number of only digits is read back as an `int`, and any other as a `float64`,
which is why a whole `float64` is written with a `.0` fraction.
History recorded before that holds a whole `float64` without one, like `42`,
so there it is read back as an `int`.

So if you store your own type in a variable or return it from a participant,
register it with the codec and it comes back as that type, even inside a
`[]any` or `map[string]any`. Forgetting to register it doesn't fail the write;
the value just comes back as a `map[string]any`:

```json
{ "$type": "workflow:event:var:set", "name": "plan", "value": { "$type": "acme:invoice", "customer_id": "cust-1" } }
```

An object with an `@type` member, like a JSON-LD document which a participant
returned, is read as a plain `map[string]any`, with the `@type` as one of its keys.

A value with a member of its own named `$type`, like a `map[string]any` with a
`"$type"` key, or a struct with a `json:"$type"` field, can't be written: it would
be read back as the value its `$type` names. Marshalling fails with
`jsonkit.ErrKeywordCollision` instead. A map with a type ID, like a
`map[string]string`, is fine, as its entries are the `$map` member of its
envelope. The codec's `TypeKeyword`, `ValueKeyword` and `MapKeyword` name the
members otherwise, see the `jsonkit` README.

History recorded before slices and maps carried their type ID holds them as plain JSON,
so there they are still read back as a `[]any` or `map[string]any`.
A `ForEach` iterates over either the same way.
Likewise, history recorded before the numbers carried their type ID holds them as plain JSON,
so there every number is still read back as a `float64`.

A value that references back to itself, like a `[]any` that contains itself,
can't be written: marshalling fails with a `*json.UnsupportedValueError`, like
with `encoding/json`.

---

## 4. The registered type catalogue

Everything `wfjson.NewCodec()` knows about, and the tag it writes:

| Go type                          | `$type`                            |
| -------------------------------- | ---------------------------------- |
| `workflow.Sequence`              | `workflow:sequence`                |
| `workflow.If`                    | `workflow:if`                      |
| `workflow.Sleep`                 | `workflow:sleep`                   |
| `workflow.For`                   | `workflow:for`                     |
| `workflow.ForEach`               | `workflow:foreach`                 |
| `workflow.Break`                 | `workflow:break`                   |
| `workflow.SetVar`                | `workflow:var:set`                 |
| `workflow.DeclareVar`            | `workflow:var:declare`             |
| `workflow.DeleteVar`             | `workflow:var:delete`              |
| `workflow.Increment`             | `workflow:op:increment`            |
| `workflow.Spawn`                 | `workflow:spawn`                   |
| `workflow.Join`                  | `workflow:join`                    |
| `workflow.Defer`                 | `workflow:defer`                   |
| `workflow.Execute`               | `workflow:execute`                 |
| `wftemplate.Condition`           | `workflow:template:condition`      |
| `workflow.EventCompleted`        | `workflow:event:completed`         |
| `workflow.EventTerminated`       | `workflow:event:terminated`        |
| `workflow.EventDeclareVar`       | `workflow:event:var:declare`       |
| `workflow.EventSetVar`           | `workflow:event:var:set`           |
| `workflow.EventDeleteVar`        | `workflow:event:var:delete`        |
| `workflow.EventParticipant`      | `workflow:event:participant`       |
| `workflow.EventCondition`        | `workflow:event:condition`         |
| `workflow.EventUseDefinition`    | `workflow:event:use-definition`    |
| `workflow.EventSpawn`            | `workflow:event:spawn`             |
| `workflow.EventJoin`             | `workflow:event:join`              |
| `workflow.EventSleepCompleted`   | `workflow:event:sleep:completed`   |
| `workflow.EventDefer`            | `workflow:event:defer`             |
| `workflow.EventForEachIteration` | `workflow:event:foreach:iteration` |
| `workflow.EventError`            | `workflow:event:error`             |
| `workflow.ExecutionRequest`      | `workflow:execution`               |
| `workflow.ProcessSchedule`       | `workflow:schedule`                |
| `workflow.ProcessCancel`         | `workflow:cancel`                  |

The workflow interfaces have a tag too. It names no value, but the element type of
a slice or map held as `any`, like `[]workflow:definition` for a `[]workflow.Definition`:

| Go type               | `$type`               |
| --------------------- | --------------------- |
| `workflow.Definition` | `workflow:definition` |
| `workflow.Condition`  | `workflow:condition`  |

The tags are compact IRIs in the sense of [JSON-LD](https://www.w3.org/TR/json-ld11/#compact-iris):
a prefix, `workflow`, and a suffix, separated by the first colon.
Until the first release, they were spelt with a double colon, like `workflow::sequence`.
`wfjson.NewCodec()` still reads those, so history written with them still decodes,
but it writes the current tags.

> Tags are part of your persisted data, so they are far harder to change than a
> Go identifier. Pick namespaced tags (`myapp:send-invoice`) for your own
> types so a future built-in cannot collide with them.

A tag is chosen when the type is registered. It is not read from a
definition's `Error()` or an event's `EventType()`, and doesn't have to match
them: `EventSetVar` is stored as `workflow:event:var:set`, while its
`EventType()` is `workflow:var:set`. The tag tells the decoder which Go type
to rebuild; `EventType()` is the constant your code switches on
(see [Glossary][GLOSSARY]).

---

## 5. Wiring it into the Runtime

```go
rt := workflow.Runtime{
	Events:        myEventRepository,
	Queue:         myQueue,
	Notifications: myBroadcast,
	Locks:         myLocks,

	Codec: wfjson.NewCodec(),
}
```

`Runtime.Codec` is optional, and today nothing in-tree reads it: the runtime
hands Go values straight to your `EventRepository`, and `adapter/memory` keeps
them as Go values. **The codec's real consumer is your persistence adapter** —
the code that has to turn a `workflow.Event` into a row or a message.

So the practical rule is: whoever writes bytes owns the codec. Pass the same
instance to your adapter that you set on the runtime, and the system has one
consistent view of the wire format — but that view is per `wfjson` version.
If you need a stored event to be readable across a `wfjson` upgrade, see §9.

---

## 6. `workflow.Codec` is deliberately tiny

```go
type Codec interface {
	codec.Codec
}

// which is, in full:
//   Marshal(v any) ([]byte, error)
//   Unmarshal(data []byte, ptr any) error
```

Nothing more. Any `port/codec.Codec` qualifies, so a YAML, CBOR, or msgpack
implementation is a drop-in — and it can be held to the same standard by the
same contract test (§8).

---

## 7. Registering your own Definition type

Your own `Definition` needs a tag too, or the codec has no way to reconstruct it.
Register it on a codec you build yourself:

```go
type SendInvoice struct {
	CustomerID string
}

func (SendInvoice) Error() string { return "SendInvoice" }

func (d SendInvoice) Execute(ctx context.Context, pid workflow.ProcessID) error {
	// ...
	return nil
}
```

```go
import "go.llib.dev/frameless/pkg/jsonkit"

// NewCodec is your app's codec: the built-ins, plus your own types.
func NewCodec() *jsonkit.Codec {
	c := wfjson.NewCodec()
	jsonkit.CodecRegisterTypeID[SendInvoice](c, "myapp:send-invoice")
	return c
}
```

That is all. `SendInvoice` now round-trips on its own and nested inside built-in
definitions:

```go
c := NewCodec()

var def workflow.Definition = workflow.Sequence{
	workflow.SetVar{Name: "customer", Value: "cust-1"},
	SendInvoice{CustomerID: "cust-1"},
}

data, _ := c.Marshal(jsonkit.Interface[workflow.Definition]{V: def})
// {"$type":"workflow:sequence","$value":[
//   {"$type":"workflow:var:set","name":"customer","value":"cust-1"},
//   {"$type":"myapp:send-invoice","customer_id":"cust-1"}]}
```

### Choose your registration

| Use                              | When                                                                                                     |
| -------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `jsonkit.CodecRegisterTypeID[T]` | The default reflect-based encoding of `T` is fine. Field names come from `T`'s own fields / `json` tags. |
| `jsonkit.CodecRegister[T]`       | You want to own the wire format — snake_case keys, a DTO shape, a schema you must stay compatible with.  |

With `CodecRegisterTypeID`, a `T` that has its own `json.Marshaler`, or else its own `encoding.TextMarshaler`,
is written with that method, like with `encoding/json`. The closures of `CodecRegister` win over both.
Define these methods with a value receiver.
A Definition is held by an interface, which isn't addressable, so a method with a pointer receiver is used only when the Definition is a `*T`.

The second form takes a `jsonkit.ITypeCodec[T]`, which is exactly the pattern
every built-in type uses:

```go
type sendInvoiceDTO struct {
	CustomerID string `json:"customer_id"`
}

type SendInvoiceCodec struct{}

func (SendInvoiceCodec) Marshal(c *jsonkit.Codec, v SendInvoice) ([]byte, error) {
	return json.Marshal(sendInvoiceDTO{CustomerID: v.CustomerID})
}

func (SendInvoiceCodec) Unmarshal(c *jsonkit.Codec, data []byte, p *SendInvoice) error {
	var dto sendInvoiceDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	p.CustomerID = dto.CustomerID
	return nil
}

func NewCodec() *jsonkit.Codec {
	c := wfjson.NewCodec()
	jsonkit.CodecRegister[SendInvoice](c, "myapp:send-invoice", SendInvoiceCodec{})
	return c
}
```

Your `Marshal` must emit **only the inner JSON** — no `$type` field. `jsonkit`
adds the envelope itself; emitting your own produces a duplicate key. For a
nested `Definition` or `Condition` field, marshal it through the passed-in
`*jsonkit.Codec` (into a `json.RawMessage`) so the inner envelope survives.

> Pick namespaced tags (`myapp:send-invoice`). They are written into every
> stored event, so a collision with a future built-in tag would be a data
> migration, not a compile error.

---

## 8. Proving a codec is correct

`wfcontract.Codec` is a ready-made suite that round-trips every registered
`Definition`, `Condition`, and `Event` — directly, through the interface, and
embedded inside `Sequence` and `If`:

```go
func TestMyCodec(t *testing.T) {
	wfcontract.Codec(wfjson.NewCodec()).Test(t)
}
```

It returns a `contract.Contract`; `.Test(t)` runs it. Point it at your own codec
to hold a new wire format to the same standard:

```go
func TestAppCodec(t *testing.T) {
	wfcontract.Codec(myapp.NewCodec()).Test(t)
}
```

Failures are reproducible: fix `TESTCASE_SEED` and rerun.

> The suite only checks the types **it** knows about. Adding your own type to a
> codec does not add it to the contract — round-trip your own types in your own
> test, the way §7 does. See [Testing][TESTING] for the wider picture.

---

## 9. Wire-format stability — the compatibility promise

`wfjson.NewCodec()` produces a wire format that is persisted to a database,
and stored event logs and definitions outlive any one version of the code. So
the package promises that what it stored keeps working after an upgrade:

- a Definition stored as JSON still decodes to the same Definition, and
- a Process that was in flight during the upgrade resumes on the new version,
  replaying its recorded work instead of repeating it.

Two test suites hold the package to it.

### The promise: `pkg/workflow/internal/promise`

Each scenario in the suite is a workflow written with the current API. Its
fixture, `testdata/<scenario>.json`, is what an earlier version stored for it:
the encoded Definition, and the event history of a Process that ran until a
participant suspended it, which stands for the upgrade. A scenario may also
stop on a failure before the upgrade, like a Process waiting on the fix that
the upgrade ships. The suite decodes the fixture with the current code and
checks that:

1. the stored Definition still decodes to the scenario's Definition, as the
   same Go value;
2. the stored history resumes, calls only the participants and conditions the
   scenario expects after the upgrade, and completes;
3. the resumed Process, and each child it spawned, ends with the variables the
   scenario expects, each of the same type it was set with, so an `int` that
   reads back as a `float64` fails it.

It checks behaviour rather than bytes, so it also catches a change that
encodes the same way but replays differently. Moving a step to another path
segment is one example: the stored calls no longer match, and they run again.

To add a scenario, append it to the scenario table, then record its fixture
and commit it:

```sh
go test ./pkg/workflow/internal/promise/ -record
```

Recording only writes the missing fixtures; it never overwrites one. A
committed fixture is never edited or recorded again, and a fixture without a
scenario fails the suite, because a promise can't be withdrawn. When the suite
fails after a change, the change breaks data that is already stored: fix the
change, for example with a codec migration, rather than the fixture.

### The pin: `pkg/workflow/wfjson/compat_test.go`

The wire bytes themselves are pinned per type. For each of the 28 types the
codec registers, the test:

1. **Builds a representative value** with stable IDs (a fixed `ProcessID`,
   `EventID`, `ChildID`, and timestamp) so the JSON output is
   byte-deterministic across runs.

2. **Asserts the marshalled bytes equal the snapshot the codec produced at
   v1**. Any rename of a JSON field, reorder of struct fields, change to an
   `$type` discriminator, or change to the `omitzero`/`omitempty` decisions
   trips this assertion with a message that points at the exact drift and
   tells the contributor how to regenerate the snapshot.

3. **Round-trips through the polymorphic interface** (`Definition`,
   `Condition`, `Event`, `Process*`) — the path production callers use — and
   asserts the unmarshal → re-marshal reproduces the same bytes. This catches
   the "codec produces JSON that decodes but doesn't re-marshal identically"
   silent-corruption shape.

4. **Asserts a `$type` envelope** is present. The polymorphic decoder relies
   on it to dispatch the value on Unmarshal; a regression that emits a bare
   value would still pass the byte-equality check for some shapes but would
   silently break the decoder.

5. **Reads the snapshot with the type IDs earlier versions wrote**, spelt with
   a double colon like `workflow::sequence`, and asserts it decodes to the same
   value.

The written format may change, as long as what was stored before still reads
the same. A tag that is only renamed, with the same shape, stays readable by
linking the former tag to the new one as an alias with
`jsonkit.CodecRegisterTypeID`, like the former double-colon tags. Replaced formats
are kept readable through codec migrations (`jsonkit.CodecRegisterMigration`),
and the pin keeps their old bytes as migration inputs. If a change intentionally
alters the written format, the failure message points at the regeneration tool:

```sh
go test -tags wfjsongen -run TestGenerateV1Snapshots ./pkg/workflow/wfjson/
```

…which writes `wfjson/v1_snapshots.txt`. Copy the new line into the
corresponding `assertMatchesV1Snapshot` call in `compat_test.go`, and commit
the change with a note that calls out the format change. The
promise fixtures stay as they are, and must still pass.

### What the promise covers

- **The built-in definitions, events and conditions**, as encoded by
  `wfjson.NewCodec()`, including the event paths their replay depends on.
- **Not your own types.** Adding a type to a codec doesn't add it to the
  promise. If your data has to outlive a version of your own code, record
  representative payloads and histories in your own repository the same way,
  and treat those as the contract for your data.

---

## 10. Where to go next

| Your question                            | Read                            |
| ---------------------------------------- | ------------------------------- |
| "What can I actually compose?"           | [Definitions][DEFINITION]       |
| "How do I test my adapter and my codec?" | [Testing][TESTING]              |
| "Why must definitions be data at all?"   | [End Users][END_USER]           |
| "What is that word again?"               | [Glossary][GLOSSARY]            |
| "I want to author a definition as JSON." | [Definitions as JSON][JSON_REF] |

[DEFINITION]: ./definition.md
[TESTING]: ./testing.md
[END_USER]: ./end-user.md
[GLOSSARY]: ./glossary.md
[JSON_REF]: ../../.agents/skills/frameless-workflow/references/definitions-as-json.md
