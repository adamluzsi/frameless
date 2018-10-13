package restresources

import (
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/queries/queryerrors"
)

func NewRESTResource(url string) *RESTResource {
	return &RESTResource{url: url}
}

type RESTResource struct{
	url string
}

func (*RESTResource) Exec(query frameless.Query) frameless.Iterator {
	switch data := query.(type) {

	case find.ByID:
		fmt.Println(data)
		return nil

	default:
		return iterators.NewError(queryerrors.ErrNotImplemented)
	}
}

func (*RESTResource) Close() error {
	return nil
}



