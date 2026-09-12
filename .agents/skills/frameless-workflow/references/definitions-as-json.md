# Definitions as JSON

> A `workflow.Definition` is just a value. The same value can be built in Go,
> serialised to JSON, shipped to a browser, edited, and decoded back. This
> reference covers what the JSON looks like for every built-in, how custom
> types slot in, and the rules that govern authoring definitions as data.

The parent skill's [Codec][CODEC] doc covers the rationale and the `wfjson`
package as a whole; this page is the reference for someone whose task is
specifically to **write or edit JSON that decodes into a `workflow.Definition`**.

[SKILL]: ../SKILL.md
[CODEC]: ../../../pkg/workflow/docs/codec.md
[DEFINITION]: ../../../pkg/workflow/docs/definition.md
[VARS]: ../../../pkg/workflow/docs/vars.md
[CONDITION]: ../../../pkg/workflow/docs/condition.md
[SIGNAL]: ../../../pkg/workflow/docs/signal.md

---

## 1. The envelope

`wfjson` wraps each polymorphic value in a type-tagged envelope:

```json
{
  "@type": "workflow::sequence",
  "@value": [ … ]
}
```

Two shapes exist, picked automatically by the codec:

| Encoded value is… | Shape                               | Example               |
| ----------------- | ----------------------------------- | --------------------- |
| a JSON object     | `@type` inlined as an extra field   | `SetVar`, `If`, `Sleep`, `Spawn` |
| anything else     | `{"@type": …, "@value": …}` wrapper | `Sequence` (an array), `wftemplate.Condition` (a string) |

You should not hand-write the envelope — write the inner value, then let the
codec wrap it. The reference examples below show both the **inner value** (what
you actually author) and the **envelope** (what the codec produces).

### Round-tripping from Go

The fastest way to learn the wire format for an unfamiliar type is to build it
in Go and marshal it:

```go
def := workflow.Sequence{
    workflow.SetVar{Name: "topic", Value: "go.llib.dev/frameless"},
    workflow.ExecuteParticipant{
        ID:     "summarise",
        Input:  []workflow.VarName{"topic"},
        Output: []workflow.VarName{"summary"},
    },
}

c := wfjson.NewCodec()
data, err := c.Marshal(def)
// {"@type":"workflow::sequence","@value":[
//   {"@type":"workflow::var::set","name":"topic","value":"go.llib.dev/frameless"},
//   {"@type":"workflow::participant","id":"summarise","input":["topic"],"output":["summary"]}
// ]}
```

For tests, parse a JSON file as a fixture and assert it round-trips:

```go
func TestOrderWorkflowFixture(t *testing.T) {
    raw, err := os.ReadFile("testdata/order.json")
    require.NoError(t, err)

    c := wfjson.NewCodec()
    var def workflow.Definition
    require.NoError(t, c.Unmarshal(raw, &def))

    // Round-trip: marshal back and assert byte-equal (or canonical-equal).
    re, err := c.Marshal(def)
    require.NoError(t, err)
    assert.JSONEq(t, string(raw), string(re))
}
```

---

## 2. The built-in catalogue

Every type the codec knows about, with its `@type` tag, the inner JSON shape,
and a worked example.

### `workflow.Sequence`

**Tag:** `workflow::sequence`
**Inner:** an array of definitions

```json
{
  "@type": "workflow::sequence",
  "@value": [
    { "@type": "workflow::var::set", "name": "topic", "value": "go.llib.dev/frameless" },
    { "@type": "workflow::participant", "id": "summarise", "input": ["topic"], "output": ["summary"] }
  ]
}
```

An empty sequence is `{"@value": []}`. A nil element in the array becomes
`null` on the wire.

### `workflow.If`

**Tag:** `workflow::if`
**Inner:** an object with `cond`, optional `then`, optional `else`

```json
{
  "@type": "workflow::if",
  "cond": { "@type": "workflow::condition", "id": "is-vip", "input": ["customer_id"] },
  "then": { "@type": "workflow::participant", "id": "apply-vip-discount", "input": ["order_id"] },
  "else": { "@type": "workflow::participant", "id": "apply-list-price",  "input": ["order_id"] }
}
```

`then` and `else` are independently optional. `cond` is required — a missing
condition is a fatal authoring error.

### `workflow.Sleep`

**Tag:** `workflow::sleep`
**Inner:** an object with optional `while` and optional `until` conditions

```json
{
  "@type": "workflow::sleep",
  "until": { "@type": "workflow::condition", "id": "approval-granted", "input": ["order_id"] }
}
```

`while` and `until` are mutually exclusive conditions on when to stop waiting.
Both are optional; with neither set the workflow suspends forever. See
[Conditions][CONDITION] for the trap about caching.

### `workflow.For`

**Tag:** `workflow::for`
**Inner:** `{init?, cond, post?, do?}`

```json
{
  "@type": "workflow::for",
  "init": { "@type": "workflow::var::set", "name": "i", "value": 0 },
  "cond": { "@type": "workflow::condition", "id": "lt", "input": ["i", "limit"] },
  "post": { "@type": "workflow::op::increment", "name": "i" },
  "do":   { "@type": "workflow::participant", "id": "process-batch-item" }
}
```

`cond` is required. The other three are independently optional.

### `workflow.ForEach`

**Tag:** `workflow::foreach`
**Inner:** `{over, do?, key?, value?}`

```json
{
  "@type": "workflow::foreach",
  "over":  "items",
  "do":    { "@type": "workflow::participant", "id": "process-item" },
  "key":   "idx",
  "value": "item"
}
```

`over` is the input collection (a `VarName`). `key` and `value` are the loop
variables written into the iteration scope; both default to nothing.

### `workflow.Break`

**Tag:** `workflow::break`
**Inner:** an empty object

```json
{ "@type": "workflow::break" }
```

Terminates the innermost `For` or `ForEach`.

### `workflow.SetVar`

**Tag:** `workflow::var::set`
**Inner:** `{name, value}`

```json
{ "@type": "workflow::var::set", "name": "topic", "value": "go.llib.dev/frameless" }
```

`value` is typed `any` — the codec preserves the JSON shape, so a number stays
a number, a string stays a string, and an object decodes into a `map[string]any`
unless the codec has a custom registration for it.

### `workflow.DeclareVar`

**Tag:** `workflow::var::declare`
**Inner:** `{name, global?}` — `global` is omitted when false

```json
{ "@type": "workflow::var::declare", "name": "trace_id", "global": true }
```

Without `global`, the binding lands in the current variable scope. With
`global: true`, it lands in the root scope and is visible everywhere.

### `workflow.DeleteVar`

**Tag:** `workflow::var::delete`
**Inner:** `{name}`

```json
{ "@type": "workflow::var::delete", "name": "card_number" }
```

There is no value on the wire — a deletion only carries the name.

### `workflow.Increment`

**Tag:** `workflow::op::increment`
**Inner:** `{name}`

```json
{ "@type": "workflow::op::increment", "name": "attempts" }
```

The increment amount is always one; there is no field to put on the wire.

### `workflow.ExecuteParticipant`

**Tag:** `workflow::participant`
**Inner:** `{id, input?, output?}`

```json
{
  "@type": "workflow::participant",
  "id":     "charge-card",
  "input":  ["order_id"],
  "output": ["receipt"]
}
```

`input` and `output` are positional lists of `VarName` strings — see
[Codec][CODEC] §5 for the wiring rule. `input` and `output` are independently
optional and omitted when empty.

### `workflow.ExecuteCondition`

**Tag:** `workflow::condition`
**Inner:** `{id, input?}`

```json
{
  "@type": "workflow::condition",
  "id":    "is-vip",
  "input": ["customer_id"]
}
```

Conditions are cached. Do not put an `ExecuteCondition` inside `Sleep` — its
answer replays forever. See [Conditions][CONDITION].

### `wftemplate.Condition`

**Tag:** `workflow::template::condition`
**Inner:** a bare string (uses the `@value` wrapper envelope)

```json
{ "@type": "workflow::template::condition", "@value": "eq .currency \"EUR\"" }
```

This is a `text/template` expression evaluated against process variables. Use
it when a workflow builder should be able to edit the rule without a
deployment. See [Conditions][CONDITION] for the function map and the
`ContextSetup` plumbing.

### `workflow.Spawn`

**Tag:** `workflow::spawn`
**Inner:** `{name, def, vars?}`

```json
{
  "@type": "workflow::spawn",
  "name": "fulfilment",
  "def":  { "@type": "workflow::participant", "id": "ship", "input": ["order"] },
  "vars": { "order_id": "order" }
}
```

`name` is the spawn identifier; it must be unique among the parent's children
and is what a later `Join` refers to. `def` is a nested definition. `vars` is
a `VarMapping` (`parent → child`) and is omitted when empty.

### `workflow.Join`

**Tag:** `workflow::join`
**Inner:** `{spawn?, collect?}`

```json
{ "@type": "workflow::join", "spawn": "fulfilment" }
```

With `spawn` set, the join waits for the named child. Without `spawn`, it
waits for every child spawned so far. `collect` is a `VarMapping` describing
which child variables to fold back into the parent; it is declared and
serialised today, but **`Join#Execute` does not yet copy anything back** — see
[Definitions][DEFINITION] §6 for the current workaround.

---

## 3. Composing the inner values

Nested definitions and conditions are encoded as already-enveloped JSON inside
the parent DTO. The codec re-uses its own codec for the nested field, so
whatever the inner type is, it appears with its own `@type`:

```json
{
  "@type": "workflow::if",
  "cond": {
    "@type": "workflow::condition",
    "id": "requires-receipt",
    "input": ["order_id"]
  },
  "then": {
    "@type": "workflow::sequence",
    "@value": [
      { "@type": "workflow::var::set",       "name": "notified", "value": true },
      { "@type": "workflow::participant",    "id": "email-receipt", "input": ["receipt"] }
    ]
  }
}
```

This composes naturally — the same patterns appear at any depth.

---

## 4. Custom `@type` registration

A custom `Definition` needs a tag, or the codec has no way to reconstruct it.
There are two registrations, picked by how much of the wire format you want
to own.

### Reflect-based: `jsonkit.CodecRegisterTypeID[T]`

Use this when the default reflection encoding of `T` is fine. Field names come
from `T`'s exported fields (or its `json` struct tags).

```go
type SendInvoice struct {
    CustomerID string
}

func (SendInvoice) Error() string { return "acme::send-invoice" }

func NewCodec() *jsonkit.Codec {
    c := wfjson.NewCodec()
    jsonkit.CodecRegisterTypeID[SendInvoice](c, "acme::send-invoice")
    return c
}
```

Wire shape (reflect-based, fields use Go names by default):

```json
{
  "@type": "acme::send-invoice",
  "CustomerID": "cust-1"
}
```

Add `json:"customer_id"` struct tags if you want snake_case on the wire.

### DTO-based: `jsonkit.CodecRegister[T]`

Use this when you need to own the wire format — DTOs, snake_case keys, a
schema you must stay compatible with. The pattern is the one every built-in
uses:

```go
type SendInvoiceDTO struct {
    CustomerID string `json:"customer_id"`
}

type SendInvoiceCodec struct{}

func (SendInvoiceCodec) Marshal(c *jsonkit.Codec, v SendInvoice) ([]byte, error) {
    return json.Marshal(SendInvoiceDTO{CustomerID: v.CustomerID})
}

func (SendInvoiceCodec) Unmarshal(c *jsonkit.Codec, data []byte, p *SendInvoice) error {
    var dto SendInvoiceDTO
    if err := json.Unmarshal(data, &dto); err != nil {
        return err
    }
    p.CustomerID = dto.CustomerID
    return nil
}

func NewCodec() *jsonkit.Codec {
    c := wfjson.NewCodec()
    jsonkit.CodecRegister[SendInvoice](c, "acme::send-invoice", SendInvoiceCodec{})
    return c
}
```

Wire shape (DTO-controlled):

```json
{ "@type": "acme::send-invoice", "customer_id": "cust-1" }
```

The rules:

- The tag must match `Error()` byte-for-byte; it is the same string twice.
- `Marshal` must emit **only the inner JSON** — no `@type` field. `jsonkit`
  adds the envelope itself; emitting your own produces a duplicate key.
- For a nested `Definition` or `Condition` field, marshal it through the
  passed-in `*jsonkit.Codec` (into a `json.RawMessage`) so the inner envelope
  survives. The built-in DTOs all do this.
- Pick namespaced tags (`acme::send-invoice`). The tag is written into every
  stored event, so a collision with a future built-in tag would be a data
  migration, not a compile error.

The same registration pattern applies to a custom `Condition` type — the
codec sees them as polymorphic values of `workflow.Condition` and dispatches
on `@type` the same way.

---

## 5. Round-tripping

Any codec that ships a definition or event must round-trip through the
polymorphic interfaces. The package's own contract test does this:

```go
func TestMyCodec(t *testing.T) {
    wfcontract.Codec(wfjson.NewCodec()).Test(t)
}
```

The contract exercises every built-in type three ways:

1. **Direct marshal/unmarshal** through the interface type.
2. **Round-trip equality** — `Unmarshal(Marshal(x)) == x` and the marshal of
   that equals the original bytes.
3. **Envelope presence** — every value gets an `@type` field.

For a custom type, add your own round-trip test alongside:

```go
func TestSendInvoiceRoundTrip(t *testing.T) {
    c := NewCodec()

    in := SendInvoice{CustomerID: "cust-1"}
    data, err := c.Marshal(workflow.Definition(in))
    require.NoError(t, err)

    var out workflow.Definition
    require.NoError(t, c.Unmarshal(data, &out))

    re, err := c.Marshal(out)
    require.NoError(t, err)
    assert.JSONEq(t, string(data), string(re))
}
```

Pin a snapshot if your data must outlive a `wfjson` upgrade — see
[Codec][CODEC] §9 for the `v1_snapshots` pattern.

---

## 6. Validation

The codec catches structural mistakes at decode time (unknown `@type`, wrong
arity, malformed JSON). It does not catch *semantic* mistakes:

- An `id` that is not registered on the runtime → `ErrParticipantNotFound` /
  `ErrConditionNotFound`, raised at execution time.
- An `input` mapping that does not match a participant's signature →
  `ErrParticipantFuncMappingMismatch`, raised at execution time.
- A `wftemplate.Condition` template that is syntactically valid but
  semantically wrong (e.g. `eq .total` with one argument) — looks fine to
  `Condition#Validate(ctx)` and fails only when the engine evaluates it.

For definitions accepted from an untrusted source, validate before binding:

```go
// Decode
var def workflow.Definition
if err := codec.Unmarshal(raw, &def); err != nil {
    return err
}

// Validate template conditions (parses without executing)
if err := workflow.ValidateDefinition(ctx, def); err != nil {
    return err
}

// Bind and run
_ = rt.Bind(ctx, pid, def)
```

`wftemplate.Condition#Validate(ctx)` parses a template without executing it.
Run it on every template condition embedded in the decoded tree (recursively,
because the condition is buried under `If`/`Sleep`).

> **Trust boundary.** Treat end-user-authored definitions as untrusted input.
> A definition can only *name* participants and conditions by ID; it cannot
> express arbitrary Go. An ID you never registered resolves to a fatal
> authoring error, and an unknown `@type` fails to decode at the boundary.
> See the [End Users][END_USER] package doc for the full rationale.

---

## 7. A worked example

An order-fulfilment flow, written in JSON by hand, that decodes into a working
`workflow.Definition`:

```json
{
  "@type": "workflow::sequence",
  "@value": [
    { "@type": "workflow::var::set", "name": "order_id", "value": "ORD-42" },

    {
      "@type": "workflow::if",
      "cond": {
        "@type": "workflow::condition",
        "id": "stock-available",
        "input": ["order_id"]
      },
      "then": {
        "@type": "workflow::sequence",
        "@value": [
          { "@type": "workflow::participant", "id": "reserve-stock", "input": ["order_id"], "output": ["reservation_id"] },
          {
            "@type": "workflow::spawn",
            "name": "fulfilment",
            "def": {
              "@type": "workflow::sequence",
              "@value": [
                { "@type": "workflow::participant", "id": "ship", "input": ["reservation_id"], "output": ["tracking"] }
              ]
            },
            "vars": { "order_id": "order" }
          }
        ]
      },
      "else": {
        "@type": "workflow::participant", "id": "backorder", "input": ["order_id"]
      }
    },

    { "@type": "workflow::join", "spawn": "fulfilment" },

    {
      "@type": "workflow::if",
      "cond": { "@type": "workflow::condition", "id": "requires-receipt", "input": ["order_id"] },
      "then": { "@type": "workflow::participant", "id": "email-receipt", "input": ["order_id", "tracking"] }
    }
  ]
}
```

A few things to notice:

- The whole tree is a `Sequence` because everything has to run in order.
- The `If` branches are themselves nested `Sequence`s — a single step is just
  a one-element sequence in disguise.
- `Spawn` declares a child process; `Join` waits for it. The parent's
  `order_id` lands in the child as `order` through the `vars` mapping.
- `input`/`output` lists on `ExecuteParticipant` are positional; the order of
  the names must match the registered participant's `func(ctx, …)` signature.
- This JSON can be parsed by `wfjson.NewCodec()` and decoded into a
  `workflow.Definition` directly — no per-type registration is needed for the
  built-ins.

---

## 8. Common mistakes

- **Hand-writing the envelope for an object type.** The codec adds `@type` for
  you. Putting it in the inner value produces a duplicate key on the way out
  and a parse error on the way in.
- **Wrapping an object in `@value`.** The codec picks the envelope shape from
  the type; a `SetVar` is an object (`@type` inlined), a `Sequence` is an
  array (`@value` wrapper). Mixing them up silently fails to decode.
- **Forgetting positional `input`/`output`.** Two participant IDs swapped
  silently if their signatures match the wiring. The arity is checked, not the
  semantics.
- **Putting `ExecuteCondition` inside `Sleep`.** The answer is cached, so the
  first `false` replays forever. Use a non-`ExecuteCondition` value for
  `Sleep#While` / `Sleep#Until`.
- **Renaming an `@type` after data has been persisted.** The codec has no
  upgrade path; old events stop decoding. Bump the tag to a new namespaced
  value and migrate via `Replace` or a one-shot rewrite.
- **Inserting or reordering steps in a `Sequence` mid-flight.** Sequence index
  is part of the path identity. Inserting at index 2 re-runs everything from
  index 2 onward. Append, or use `Replace`.
- **Custom types without a registration.** Decoding fails with an
  `unknown @type` error. Register once on the codec used by the persistence
  adapter.

---

## 9. Where to go next

| Your question                                  | Read                          |
| ---------------------------------------------- | ----------------------------- |
| "Why are definitions data at all?"             | [End Users][END_USER]        |
| "What can I compose, in Go?"                   | [Definitions][DEFINITION]     |
| "How do variables and scoping actually work?"  | [Variables][VARS]             |
| "How do conditions evaluate?"                  | [Conditions][CONDITION]       |
| "What can I pause, stop, or swap?"             | [Signals][SIGNAL]             |
| "Codec round-trip contract tests"              | [Codec][CODEC] §8             |
| "Authoring a custom `Definition` type"         | [Custom Definition][CUSTOM_DEFINITION] |

[CUSTOM_DEFINITION]: ../../../pkg/workflow/docs/custom-definition.md
[END_USER]: ../../../pkg/workflow/docs/end-user.md
