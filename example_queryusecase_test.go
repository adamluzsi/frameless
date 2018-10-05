package frameless_test

import (
	"github.com/adamluzsi/frameless/queries/save"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

type User struct {
	IsActive bool
}

type InactiveUsers struct{} // <- Query

// Remove extra T from Test, it is only added here so the full page example can work in godoc
func (quc InactiveUsers) TTest(suite *testing.T, storage frameless.Storage, resetStorage func()) {
	suite.Run("dependency", func(t *testing.T) {
		save.Entity{Entity: &User{}}.Test(t, storage, resetStorage)
	})

	suite.Run("Query For Inactive Users", func(spec *testing.T) {
		defer resetStorage()

		spec.Log("Given 10 users stored in the storage")
		inactiveUsers := []*User{}
		for i := 0; i < 10; i++ {
			u := &User{IsActive: i < 7}

			if !u.IsActive {
				inactiveUsers = append(inactiveUsers, u)
			}

			require.Nil(suite, storage.Exec(save.Entity{Entity: u}).Err())
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
	})
}

type UsersNameBeginWith struct{ Prefix string }

func ExampleQueryUseCase() {}
