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
	workflow.ExecuteParticipant{ID: "greet", Input: []workflow.VarName{"name"}},
}

data, err := c.Marshal(def)

var got workflow.Definition
err = c.Unmarshal(data, &got) // a workflow.Sequence again, not a map[string]any
```

`wfjson.NewCodec()` returns a **fresh** `*jsonkit.Codec` on every call. Each one
owns a private registry of type tags, so two codecs never leak registrations into
each other — which is also why the contract suite can hand every scenario its own
instance.

---

## 3. What the wire looks like

`jsonkit` wraps each polymorphic value in a **type-tagged envelope** keyed on
`@type`:

```json
{
  "@type": "workflow::sequence",
  "@value": [
    { "@type": "workflow::var::set", "name": "name", "value": "World" },
    { "@type": "workflow::participant", "id": "greet", "input": ["name"] }
  ]
}
```

Two envelope shapes exist, picked automatically:

| Encoded value is… | Shape                               | Example               |
| ----------------- | ----------------------------------- | --------------------- |
| a JSON object     | `@type` inlined as an extra field   | `SetVar`, `If`        |
| anything else     | `{"@type": …, "@value": …}` wrapper | `Sequence` (an array) |

The field names inside the envelope (`name`, `id`, `input`, …) come from private
DTO structs in `wfjson`, **not** from struct tags on the `workflow` types. That
is deliberate: the `workflow` package stays free of any JSON knowledge, and the
wire format can evolve without touching the domain types.

---

## 4. The registered type catalogue

Everything `wfjson.NewCodec()` knows about, and the tag it writes:

| Go type                       | `@type`                          |
| ----------------------------- | -------------------------------- |
| `workflow.Sequence`           | `workflow::sequence`             |
| `workflow.If`                 | `workflow::if`                   |
| `workflow.Sleep`              | `workflow::sleep`                |
| `workflow.For`                | `workflow::for`                  |
| `workflow.ForEach`            | `workflow::foreach`              |
| `workflow.Break`              | `workflow::break`                |
| `workflow.SetVar`             | `workflow::var::set`             |
| `workflow.DeclareVar`         | `workflow::var::declare`         |
| `workflow.DeleteVar`          | `workflow::var::delete`          |
| `workflow.Increment`          | `workflow::op::increment`        |
| `workflow.Spawn`              | `workflow::spawn`                |
| `workflow.Join`               | `workflow::join`                 |
| `workflow.ExecuteParticipant` | `workflow::participant`          |
| `workflow.ExecuteCondition`   | `workflow::condition`            |
| `wftemplate.Condition`        | `workflow::template::condition`  |
| `workflow.EventCompleted`     | `workflow::event::completed`     |
| `workflow.EventTerminated`    | `workflow::event::terminated`    |
| `workflow.EventDeclareVar`    | `workflow::event::var::declare`  |
| `workflow.EventSetVar`        | `workflow::event::var::set`      |
| `workflow.EventDeleteVar`     | `workflow::event::var::delete`   |
| `workflow.EventParticipant`   | `workflow::event::participant`   |
| `workflow.EventCondition`     | `workflow::event::condition`     |
| `workflow.EventUseDefinition` | `workflow::event::use-definition`|
| `workflow.EventSpawn`         | `workflow::event::spawn`         |
| `workflow.EventJoin`          | `workflow::event::join`          |
| `workflow.ProcessExecution`   | `workflow::execution`            |
| `workflow.ProcessSchedule`    | `workflow::schedule`             |
| `workflow.ProcessCancel`      | `workflow::cancel`               |

> Tags are part of your persisted data, so they are far harder to change than a
> Go identifier. Pick namespaced tags (`myapp::send-invoice`) for your own
> types so a future built-in cannot collide with them.

---

## 5. Wiring it into the Runtime

```go
rt := workflow.Runtime{
	Events:                 myEventRepository,
	ProcessExecutionQueue:  myQueue,
	ProcessChangeBroadcast: myBroadcast,
	ProcessLockers:         myLocks,

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
	jsonkit.CodecRegisterTypeID[SendInvoice](c, "myapp::send-invoice")
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

data, _ := c.Marshal(def)
// {"@type":"workflow::sequence","@value":[
//   {"@type":"workflow::var::set","name":"customer","value":"cust-1"},
//   {"@type":"myapp::send-invoice","customer_id":"cust-1"}]}
```

### Choose your registration

| Use                              | When                                                                                                     |
| -------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `jsonkit.CodecRegisterTypeID[T]` | The default reflect-based encoding of `T` is fine. Field names come from `T`'s own fields / `json` tags. |
| `jsonkit.CodecRegister[T]`       | You want to own the wire format — snake_case keys, a DTO shape, a schema you must stay compatible with.  |

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
	jsonkit.CodecRegister[SendInvoice](c, "myapp::send-invoice", SendInvoiceCodec{})
	return c
}
```

Your `Marshal` must emit **only the inner JSON** — no `@type` field. `jsonkit`
adds the envelope itself; emitting your own produces a duplicate key. For a
nested `Definition` or `Condition` field, marshal it through the passed-in
`*jsonkit.Codec` (into a `json.RawMessage`) so the inner envelope survives.

> Pick namespaced tags (`myapp::send-invoice`). They are written into every
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

## 9. Wire-format stability — best effort, with a pin

`wfjson.NewCodec()` produces a wire format that is persisted to a database,
and stored event logs and definitions outlive any one version of the code. We
*try* to keep that format stable across releases, but **we do not promise
it**: the JSON shape may change as the package evolves, and any byte you store
today should be treated as readable only by the codec that wrote it. The
section is here because we want the intent to be visible — *we are trying*
— and because accidental drift (a field rename, an `@type` discriminator
typo, a struct reorder that shifts the JSON keys) is the shape of breakage we
most want to avoid.

The wire format is pinned by `pkg/workflow/wfjson/compat_test.go`. For each of
the 28 types the codec registers, the test:

1. **Builds a representative value** with stable IDs (a fixed `ProcessID`,
   `EventID`, `ChildID`, and timestamp) so the JSON output is
   byte-deterministic across runs.

2. **Asserts the marshalled bytes equal the snapshot the codec produced at
   v1**. Any rename of a JSON field, reorder of struct fields, change to an
   `@type` discriminator, or change to the `omitzero`/`omitempty` decisions
   trips this assertion with a message that points at the exact drift and
   tells the contributor how to regenerate the snapshot.

3. **Round-trips through the polymorphic interface** (`Definition`,
   `Condition`, `Event`, `Process*`) — the path production callers use — and
   asserts the unmarshal → re-marshal reproduces the same bytes. This catches
   the "codec produces JSON that decodes but doesn't re-marshal identically"
   silent-corruption shape.

4. **Asserts an `@type` envelope** is present. The polymorphic decoder relies
   on it to dispatch the value on Unmarshal; a regression that emits a bare
   value would still pass the byte-equality check for some shapes but would
   silently break the decoder.

If a future change intentionally widens the format — adding a field, renaming
a discriminator — the failure message points at the regeneration tool:

```sh
go test -tags wfjsongen -run TestGenerateV1Snapshots ./pkg/workflow/wfjson/
```

…which writes `wfjson/v1_snapshots.txt`. Copy the new line into the
corresponding `assertMatchesV1Snapshot` call in `compat_test.go`, and commit
the change with a note that calls out the format change.

### What the test is for, and what it is not

- **It catches accidental drift.** An engineer who renames `event_id` to
  `id` without thinking breaks the build before the change merges. That is
  the test's job.

- **It is *not* a backwards-compatibility contract.** A deliberate change
  to the wire format is allowed: update the snapshot, ship the change. If
  you store events with one version of the codec and read them back with
  another, both versions must include the same `wfjson` package version.
  There is no on-disk version stamp in the envelope; *that* is the contract.

- **If your data needs to outlive `wfjson`, pin it yourself.** Snapshot a
  representative payload of each type your system actually uses into a
  contract test in your own repository, and treat *that* as the contract for
  your data. The `wfjson` snapshots are a backstop against careless
  refactors, not a publication of a stable schema.

---

## 10. Where to go next

| Your question                            | Read                      |
| ---------------------------------------- | ------------------------- |
| "What can I actually compose?"           | [Definitions][DEFINITION] |
| "How do I test my adapter and my codec?" | [Testing][TESTING]        |
| "Why must definitions be data at all?"   | [End Users][END_USER]     |
| "What is that word again?"               | [Glossary][GLOSSARY]      |

[DEFINITION]: ./definition.md
[TESTING]: ./testing.md
[END_USER]: ./end-user.md
[GLOSSARY]: ./glossary.md
