package httpkit_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"

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

func (xid XID) Int() int {
	return int(xid)
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

type BazENTID int

type BazENT struct {
	ID  BazENTID
	Baz int
}

type BazDTO struct {
	ID  BazENTID `json:"id"`
	Baz int      `json:"baz"`
}

func MakeBazMapping() BazENTMapping {
	return BazENTMapping{
		IDConverter: httpkit.IDConverter[int]{
			Format: func(id int) (string, error) {
				return strconv.Itoa(id), nil
			},
			Parse: strconv.Atoi,
		},
	}
}

type BazENTMapping struct {
	httpkit.IDConverter[int]
	httpkit.IDInContext[BazENTMapping, string]
}

func (f BazENTMapping) MapEntity(ctx context.Context, dto BazDTO) (BazENT, error) {
	return BazENT{ID: dto.ID, Baz: dto.Baz}, nil
}

func (f BazENTMapping) MapDTO(ctx context.Context, entity BazENT) (BazDTO, error) {
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
