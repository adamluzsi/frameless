package postgresql

import "fmt"

func makePrepareStatementPlaceholderGenerator() func() string {
	var index = 0
	return func() string {
		index++
		return fmt.Sprintf(`$%d`, index)
	}
}
