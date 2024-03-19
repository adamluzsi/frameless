package querykit_test

import (
	"fmt"
	"go.llib.dev/frameless/pkg/querykit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/assert"
	"testing"
)

func TestVisit(t *testing.T) {
	var node querykit.Node
	node = querykit.And{
		Left: querykit.Compare{
			Left:  querykit.Field[testent.Foo]{Name: "foo"},
			Right: querykit.Value{Value: "42"},
		},
		Right: querykit.Compare{
			Left:  querykit.Field[testent.Foo]{Name: "bar"},
			Right: querykit.Value{Value: "The Answer."},
		},
	}

	var visitor querykit.Visitor[string]
	visitor = func(node querykit.Node) (string, error) {
		switch node := node.(type) {
		case querykit.Field[testent.Foo]:
			return fmt.Sprintf("%q", node.Name), nil // map name to column name

		case querykit.Field[testent.Bar]:
			return fmt.Sprintf("%q", node.Name), nil // map name to column name

		case querykit.Value:
			switch val := node.Value.(type) {
			case string:
				return fmt.Sprintf(`'%s'`, val), nil
			default:
				return "", fmt.Errorf("not implemented")
			}

		case querykit.And:
			l, err := visitor(node.Left)
			if err != nil {
				return "", err
			}
			r, err := visitor(node.Right)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s AND %s", l, r), nil

		case querykit.Compare:
			if err := node.Validate(); err != nil {
				return "", err
			}
			l, err := visitor(node.Left)
			if err != nil {
				return "", err
			}
			r, err := visitor(node.Right)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s AND %s", l, r), nil
		default:
			return "", fmt.Errorf("not implemented")
		}
	}

	query, err := visitor(node)
	assert.NoError(t, err)
	assert.Equal(t, `"foo" AND '42' AND "bar" AND 'The Answer.'`, query)
}
