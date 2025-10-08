# UUID

## V7

UUID v7 is monotonic within a process and ordered by time, making it ideal for databases and distributed systems.

```go
id, err := uuid.MakeV7()
```

| Field      | Bit Positions | Byte Positions                 | Length (bits) | Description                                                                           |
| ---------- | ------------- | ------------------------------ | ------------- | ------------------------------------------------------------------------------------- |
| unix_ts_ms | 0–47          | uuid[0:6]                      | 48            | 48-bit big-endian unsigned integer representing milliseconds since the Unix Epoch.    |
| ver        | 48–51         | uuid[6] (high nibble)          | 4             | 4-bit version field, fixed to `0111` (7).                                             |
| rand_a     | 52–63         | uuid[6] (low nibble) – uuid[7] | 12            | 12-bit field for additional time precision or monotonic counter within a millisecond. |
| var        | 64–65         | uuid[8] (high 2 bits)          | 2             | 2-bit variant field, fixed to `10` to conform with RFC 4122.                          |
| rand_b     | 66–127        | uuid[8:16]                     | 62            | 62-bit cryptographically secure random field (from `crypto/rand`).                    |

---

## V4

UUID v4 is completely random. It provides no ordering or time-based semantics — only uniqueness with extremely low collision probability (1 in 2^122).

```go
id, err := uuid.MakeV4()
```


| Field | Bit Positions | Byte Positions                   | Length (bits) | Description                                                    |
| ----- | ------------- | -------------------------------- | ------------- | -------------------------------------------------------------- |
| ver   | 48–51         | uuid[6] (high nibble)            | 4             | 4-bit version field, fixed to `0100` (4).                      |
| var   | 64–65         | uuid[8] (high 2 bits)            | 2             | 2-bit variant field, fixed to `10` to conform with RFC 4122.   |
| rand  | All others    | uuid[0:6], uuid[7:8], uuid[9:16] | 122           | 122 cryptographically secure random bits (from `crypto/rand`). |

---

## Errors

When the Go runtime could not read random data from `crypto/rand`, it usually means:

- You're running in a locked-down environment (like Docker or a VM) without `/dev/urandom`
- The system is out of entropy (common on headless servers or cloud instances)
- `/dev/urandom` is missing, unreadable, or blocked by a security policy
- You're on an old or broken OS with no proper random number generator

> The `uuid` package require cryptographically secure randomness.
> If `crypto/rand` fails, none of the UUID generation will be possible,
> unless you provide your own Random `io.reader` in the generators.
