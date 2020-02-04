package fixtures_test

import (
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomString(t *testing.T) {

	// TODO fix this
	if os.Getenv("SKIP_CRYPTO_RAND_TEST") == "TRUE" {
		t.Skip()
	}

	t.Run("when random string requested", func(t *testing.T) {
		t.Run("it is expected to return one", func(t *testing.T) {

			ExpectedLength := rand.New(rand.NewSource(time.Now().Unix())).Intn(42) + 100

			str := fixtures.RandomString(ExpectedLength)

			t.Run("and the received string length is expected to be just as much as the input parameter requested", func(t *testing.T) {
				require.Equal(t, ExpectedLength, len(str))
			})

			t.Run("and the received string should be random and not repeating", func(t *testing.T) {

				var wg sync.WaitGroup

				for i := 0; i < 1024; i++ {
					wg.Add(1)

					go func() {
						defer wg.Done()
						othStr := fixtures.RandomString(ExpectedLength)
						assert.NotEqual(t, str, othStr)
					}()
				}

				wg.Wait()

				if t.Failed() {
					t.FailNow()
				}
			})

		})
	})
}
