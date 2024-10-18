# draft

how would you call when a json scanner that finds a valid raw encoded entity part within a json?
In this example, I just called that as `json` field

For example, in this sample:
```json
[{"key1":"value1","ary":[1, 2, 3]}]
```


the scanner would find:
- path: ["array", "array-value", "object", "object-key"]
  raw: []byte("key1")
  kind: "string"
- path: ["array", "array-value", "object", "object-value"]
  raw: []byte("value1")
  kind: "string"
  key: "key1"
- path: ["array", "array-value", "object", "object-key"]
  raw: []byte("ary")
  kind: "string"
- path: ["array", "array-value", "object", "object-value"]
  raw: []byte("[1, 2, 3]")
  kind: "array"
  key: "ary"
- path: ["array", "array-value", "object", "object-value", "array", "array-value"]
  raw: []byte("1")
  kind: "number"
  index: 0
- path: ["array", "array-value", "object", "object-value", "array", "array-value"]
  raw: []byte("2")
  kind: "number"
  index: 1
- path: ["array", "array-value", "object", "object-value", "array", "array-value"]
  raw: []byte("3")
  kind: "number"
  index: 2
- path: ["array", "array-value"]
  kind: "object"
  raw: []byte(`{"key1":"value1","ary":[1, 2, 3]}`)
- path: []
  kind: "array"
  raw: []byte(`[{"key1":"value1","ary":[1, 2, 3]}]`)
