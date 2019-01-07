package frameless_test

import (
	"github.com/adamluzsi/frameless/queries"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type User struct {
	IsActive bool
}

type InactiveUsers struct{} // <- Query

// Remove extra T from Test, it is only added here so the full page example can work in godoc
func (quc InactiveUsers) TTest(suite *testing.T, storage frameless.Resource) {
	suite.Run("dependency", func(t *testing.T) {
		queries.Save{Entity: &User{}}.Test(t, storage)
	})

	suite.Run("Query For Inactive Users", func(spec *testing.T) {

		var inactiveUsers []*User

		spec.Log("Given 10 users stored in the storage")
		for i := 0; i < 10; i++ {
			u := &User{IsActive: i < 7}

			if !u.IsActive {
				inactiveUsers = append(inactiveUsers, u)
			}

			require.Nil(suite, storage.Exec(queries.Save{Entity: u}).Err())
		}

		suite.Run("All Inactive users returned on search", func(t *testing.T) {

			i := storage.Exec(InactiveUsers{})
			require.Nil(t, i.Err())

			count := 0

			for i.Next() {
				count++
				var user User
				i.Decode(&user)
				require.Contains(t, inactiveUsers, &user)
			}

			require.Equal(t, len(inactiveUsers), count)
		})

		for _, u := range inactiveUsers {
			id, ok := queries.LookupID(u)
			require.True(spec, ok)
			require.NotEmpty(spec, id)
			storage.Exec(queries.DeleteByID{Type: *u, ID: id})
		}
	})
}

func ExampleQuery() {}
