package fixtures

import (
	"math/rand"
	"reflect"
	"sync"
	"time"
)

var Random = NewRandomizer(rand.NewSource(time.Now().Unix()))

func NewRandomizer(s rand.Source) *Randomizer {
	return &Randomizer{Source: s}
}

type Randomizer struct {
	Source rand.Source

	m sync.Mutex
}

// Int returns a non-negative pseudo-random int.
func (r *Randomizer) Int() int {
	r.m.Lock()
	defer r.m.Unlock()
	return rand.New(r.Source).Int()
}

// IntN returns, as an int, a non-negative pseudo-random number in [0,n).
// It panics if n <= 0.
func (r *Randomizer) IntN(n int) int {
	r.m.Lock()
	defer r.m.Unlock()
	return rand.New(r.Source).Intn(n)
}

// Float32 returns, as a float32, a pseudo-random number in [0.0,1.0).
func (r *Randomizer) Float32() float32 {
	r.m.Lock()
	defer r.m.Unlock()
	return rand.New(r.Source).Float32()
}

// Float64 returns, as a float64, a pseudo-random number in [0.0,1.0).
func (r *Randomizer) Float64() float64 {
	r.m.Lock()
	defer r.m.Unlock()
	return rand.New(r.Source).Float64()
}

// IntBetween returns, as an int, a non-negative pseudo-random number based on the received int range's [min,max].
func (r *Randomizer) IntBetween(min, max int) int {
	return min + r.IntN((max+1)-min)
}

func (r *Randomizer) ElementFromSlice(slice interface{}) interface{} {
	s := reflect.ValueOf(slice)
	index := rand.New(r.Source).Intn(s.Len())
	return s.Index(index).Interface()
}

func (r *Randomizer) KeyFromMap(anyMap interface{}) interface{} {
	s := reflect.ValueOf(anyMap)
	index := rand.New(r.Source).Intn(s.Len())
	return s.MapKeys()[index].Interface()
}

func (r *Randomizer) Bool() bool {
	return r.IntN(2) == 0
}

func (r *Randomizer) String() string {
	return r.StringN(r.IntBetween(4, 42))
}

func (r *Randomizer) StringN(length int) string {
	r.m.Lock()
	defer r.m.Unlock()

	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

	bytes := make([]byte, length)
	if _, err := rand.New(r.Source).Read(bytes); err != nil {
		panic(err)
	}

	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}

	return string(bytes)
}

// TimeBetween returns, as an time.Time, a non-negative pseudo-random time in [from,to].
func (r *Randomizer) TimeBetween(from, to time.Time) time.Time {
	return time.Unix(int64(r.IntBetween(int(from.Unix()), int(to.Unix()))), 0).UTC()
}

func (r *Randomizer) Time() time.Time {
	t := time.Now().UTC()
	from := t.AddDate(0, 0, r.IntN(42)*-1)
	to := t.AddDate(0, 0, r.IntN(42)).Add(time.Second)
	return r.TimeBetween(from, to)
}
