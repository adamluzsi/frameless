package vault_test

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/testcase/random"
)

type Entity[ID any] struct {
	ID     ID `ext:"id"`
	String string
	Int    int
	Float  float64
	Bool   bool
	Slice  []string
	Map    map[string]int
	Time   time.Time
}

func MakeEntity[ID any](rnd *random.Random) Entity[ID] {
	return Entity[ID]{
		String: rnd.String(),
		Int:    rnd.Int(),
		Float:  rnd.Float64(),
		Bool:   rnd.Bool(),
		Slice:  random.Slice(rnd.IntBetween(0, 7), rnd.String),
		Map:    random.Map(rnd.IntBetween(0, 7), func() (string, int) { return rnd.String(), rnd.Int() }),
		Time:   rnd.Time(),
	}
}

func ChangeEntity[ID any](rnd *random.Random, ptr *Entity[ID]) {
	ptr.String = rnd.String()
	ptr.Int = rnd.Int()
	ptr.Float = rnd.Float64()
	ptr.Bool = rnd.Bool()
	ptr.Slice = random.Slice(rnd.IntBetween(0, 7), rnd.String)
	ptr.Map = random.Map(rnd.IntBetween(0, 7), func() (string, int) { return rnd.String(), rnd.Int() })
	ptr.Time = rnd.Time()
}

type EntityJSONDTO[ID any] struct {
	ID     ID             `json:"id"`
	String string         `json:"string"`
	Int    int            `json:"int"`
	Float  float64        `json:"float"`
	Bool   bool           `json:"bool"`
	Slice  []string       `json:"slice"`
	Map    map[string]int `json:"map"`
	Time   string         `json:"time"`
}

func EntityToEntityJSONDTOMapping[ID ~string]() dtokit.Mapping[Entity[ID], EntityJSONDTO[ID]] {
	return dtokit.Mapping[Entity[ID], EntityJSONDTO[ID]]{
		ToENT: func(ctx context.Context, dto EntityJSONDTO[ID]) (Entity[ID], error) {
			var tm time.Time
			if len(dto.Time) != 0 {
				var err error
				tm, err = time.Parse(time.RFC3339Nano, dto.Time)
				if err != nil {
					return Entity[ID]{}, err
				}
			}
			return Entity[ID]{
				ID:     dto.ID,
				String: dto.String,
				Int:    dto.Int,
				Float:  dto.Float,
				Bool:   dto.Bool,
				Slice:  dto.Slice,
				Map:    dto.Map,
				Time:   tm,
			}, nil
		},
		ToDTO: func(ctx context.Context, ent Entity[ID]) (EntityJSONDTO[ID], error) {
			var tm string
			if !ent.Time.IsZero() {
				tm = ent.Time.Format(time.RFC3339Nano)
			}
			return EntityJSONDTO[ID]{
				ID:     ent.ID,
				String: ent.String,
				Int:    ent.Int,
				Float:  ent.Float,
				Bool:   ent.Bool,
				Slice:  ent.Slice,
				Map:    ent.Map,
				Time:   tm,
			}, nil
		},
	}
}
