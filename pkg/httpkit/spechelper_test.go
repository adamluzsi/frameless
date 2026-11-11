package httpkit_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"

	"go.llib.dev/frameless/pkg/httpkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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

// commitManagerStub wraps a commit manager to make BeginTx or CommitTx fail on demand.
type commitManagerStub struct {
	comproto.OnePhaseCommitProtocol
	BeginTxErr  error
	CommitTxErr error
}

func (cm *commitManagerStub) BeginTx(ctx context.Context) (context.Context, error) {
	if cm.BeginTxErr != nil {
		return ctx, cm.BeginTxErr
	}
	return cm.OnePhaseCommitProtocol.BeginTx(ctx)
}

func (cm *commitManagerStub) CommitTx(ctx context.Context) error {
	if cm.CommitTxErr != nil {
		// a failed commit leaves no changes behind
		_ = cm.OnePhaseCommitProtocol.RollbackTx(ctx)
		return cm.CommitTxErr
	}
	return cm.OnePhaseCommitProtocol.CommitTx(ctx)
}

func respondsWithJSON[DTO any](t *testcase.T, recorder *httptest.ResponseRecorder) DTO {
	t.Helper()
	var dto DTO
	t.Log("body:", recorder.Body.String())
	assert.Must(t).NotEmpty(recorder.Body.Bytes())
	assert.Must(t).NoError(json.Unmarshal(recorder.Body.Bytes(), &dto))
	return dto
}
