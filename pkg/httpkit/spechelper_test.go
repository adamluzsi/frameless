package httpkit_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/testcase"
)

type (
	O struct {
		ID OID
	}
	OID int
	X   struct {
		ID  XID
		N   int
		OID OID
	}
	XID int
)

type XDTO struct {
	ID  int `json:"id"`
	X   int `json:"xnum"`
	OID int `json:"oid"`
}

var _ = dtokit.Register[X, XDTO](XMapping{}.ToDTO, XMapping{}.ToEnt)

type XMapping struct {
	httpkit.IntID[XID]
	httpkit.IDInContext[XMapping, XID]
}

func (f XMapping) ToEnt(ctx context.Context, dto XDTO) (X, error) {
	return X{ID: XID(dto.ID), N: dto.X, OID: OID(dto.OID)}, nil
}

func (f XMapping) ToDTO(ctx context.Context, ent X) (XDTO, error) {
	return XDTO{ID: int(ent.ID), X: ent.N, OID: int(ent.OID)}, nil
}

func (f XMapping) MapEntity(ctx context.Context, dto XDTO) (X, error) {
	return X{ID: XID(dto.ID), N: dto.X, OID: OID(dto.OID)}, nil
}

func (f XMapping) MapDTO(ctx context.Context, entity X) (XDTO, error) {
	return XDTO{ID: int(entity.ID), X: entity.N, OID: int(entity.OID)}, nil
}

type Y struct {
	ID string
	C  int
}

type YDTO struct {
	ID string `json:"id"`
	C  int    `json:"count"`
}

type YMapping struct {
	httpkit.StringID[string]
	httpkit.IDInContext[YMapping, string]
}

func (f YMapping) MapEntity(ctx context.Context, dto YDTO) (Y, error) {
	return Y{ID: dto.ID, C: dto.C}, nil
}

func (f YMapping) MapDTO(ctx context.Context, entity Y) (YDTO, error) {
	return YDTO{ID: entity.ID, C: entity.C}, nil
}

type BazID int

type Baz struct {
	ID  BazID
	Baz int
}

type BazDTO struct {
	ID  BazID `json:"id"`
	Baz int   `json:"baz"`
}

func MakeBazMapping() BazMapping {
	return BazMapping{
		IDConverter: httpkit.IDConverter[int]{
			Format: func(id int) (string, error) {
				return strconv.Itoa(id), nil
			},
			Parse: strconv.Atoi,
		},
	}
}

type BazMapping struct {
	httpkit.IDConverter[int]
	httpkit.IDInContext[BazMapping, string]
}

func (f BazMapping) MapEntity(ctx context.Context, dto BazDTO) (Baz, error) {
	return Baz{ID: dto.ID, Baz: dto.Baz}, nil
}

func (f BazMapping) MapDTO(ctx context.Context, entity Baz) (BazDTO, error) {
	return BazDTO{ID: entity.ID, Baz: entity.Baz}, nil
}

func respondsWithJSON[DTO any](t *testcase.T, recorder *httptest.ResponseRecorder) DTO {
	t.Helper()
	var dto DTO
	t.Log("body:", recorder.Body.String())
	t.Must.NotEmpty(recorder.Body.Bytes())
	t.Must.NoError(json.Unmarshal(recorder.Body.Bytes(), &dto))
	return dto
}
