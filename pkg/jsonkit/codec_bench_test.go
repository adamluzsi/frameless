package jsonkit_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/jsonkit"
)

type benchShape interface{ benchArea() float64 }

type benchCircle struct {
	Radius float64 `json:"radius"`
}

func (c benchCircle) benchArea() float64 { return 3.14159 * c.Radius * c.Radius }

type benchRect struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func (r benchRect) benchArea() float64 { return r.W * r.H }

type benchItem struct {
	ID        int               `json:"id"`
	Name      string            `json:"name"`
	Tags      []string          `json:"tags,omitempty"`
	Price     float64           `json:"price"`
	CreatedAt time.Time         `json:"created_at"`
	Attrs     map[string]string `json:"attrs"`
}

// benchPlainDoc has no interface in it, so encoding/json writes and reads it the very same way as the Codec.
type benchPlainDoc struct {
	Title string      `json:"title"`
	Items []benchItem `json:"items"`
}

// benchPolyDoc holds values in interfaces, which the Codec writes with their $type.
type benchPolyDoc struct {
	Title  string         `json:"title"`
	Items  []benchItem    `json:"items"`
	Main   benchShape     `json:"main"`
	Shapes []benchShape   `json:"shapes"`
	Meta   map[string]any `json:"meta"`
}

const benchSize = 50

func makeBenchItems() []benchItem {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	items := make([]benchItem, benchSize)
	for i := range items {
		items[i] = benchItem{
			ID:        i,
			Name:      "item-" + strconv.Itoa(i),
			Tags:      []string{"a", "b", "c"},
			Price:     float64(i) * 1.25,
			CreatedAt: created.Add(time.Duration(i) * time.Hour),
			Attrs:     map[string]string{"colour": "red", "size": "L"},
		}
	}
	return items
}

func makeBenchPlainDoc() benchPlainDoc {
	return benchPlainDoc{Title: "plain", Items: makeBenchItems()}
}

func makeBenchPolyDoc() benchPolyDoc {
	shapes := make([]benchShape, benchSize)
	for i := range shapes {
		if i%2 == 0 {
			shapes[i] = benchCircle{Radius: float64(i)}
		} else {
			shapes[i] = benchRect{W: float64(i), H: float64(i + 1)}
		}
	}
	return benchPolyDoc{
		Title:  "poly",
		Items:  makeBenchItems(),
		Main:   benchCircle{Radius: 1},
		Shapes: shapes,
		Meta: map[string]any{
			"owner":   "bench",
			"count":   benchSize,
			"enabled": true,
			"circle":  benchCircle{Radius: 2},
			"rect":    benchRect{W: 3, H: 4},
		},
	}
}

func makeBenchCodec(tb testing.TB) *jsonkit.Codec {
	var c jsonkit.Codec
	tb.Cleanup(jsonkit.CodecRegisterTypeID[benchCircle](&c, "bench:circle"))
	tb.Cleanup(jsonkit.CodecRegisterTypeID[benchRect](&c, "bench:rect"))
	return &c
}

func BenchmarkCodec(b *testing.B) {
	b.Run("plain", func(b *testing.B) {
		c := makeBenchCodec(b)
		doc := makeBenchPlainDoc()
		data, err := json.Marshal(doc)
		if err != nil {
			b.Fatal(err)
		}

		b.Run("Marshal", func(b *testing.B) {
			b.Run("encoding/json", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := json.Marshal(doc); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Codec", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := c.Marshal(doc); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
		b.Run("Unmarshal", func(b *testing.B) {
			b.Run("encoding/json", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var got benchPlainDoc
					if err := json.Unmarshal(data, &got); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Codec", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var got benchPlainDoc
					if err := c.Unmarshal(data, &got); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	})
	b.Run("interfaces", func(b *testing.B) {
		c := makeBenchCodec(b)
		doc := makeBenchPolyDoc()
		data, err := c.Marshal(doc)
		if err != nil {
			b.Fatal(err)
		}

		b.Run("Marshal", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := c.Marshal(doc); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run("Unmarshal", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var got benchPolyDoc
				if err := c.Unmarshal(data, &got); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}
