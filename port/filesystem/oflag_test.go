package filesystem_test

import (
	"os"
	"strconv"
	"testing"

	ffs "go.llib.dev/frameless/port/filesystem"

	"go.llib.dev/testcase/assert"
)

func TestBitwise(t *testing.T) {
	it := assert.MakeIt(t)
	a, err := strconv.ParseInt("1000", 2, 64)
	assert.NoError(it, err)
	b, err := strconv.ParseInt("0100", 2, 64)
	assert.NoError(it, err)
	t.Log(strconv.FormatInt(a|b, 2))
	t.Log(strconv.FormatInt((a|b)&a, 2))

	t.Log(strconv.FormatInt(int64(os.O_RDWR), 2))
	t.Log(strconv.FormatInt(int64(os.O_WRONLY), 2))
	t.Log(strconv.FormatInt(int64(os.O_RDONLY), 2))

	t.Log("has read")
	t.Log(strconv.FormatInt(int64(os.O_RDWR&os.O_RDONLY), 2))
	t.Log(strconv.FormatInt(int64(os.O_RDWR&os.O_RDWR), 2))
	t.Log(strconv.FormatInt(int64(os.O_RDONLY&os.O_RDONLY), 2))
	t.Log(strconv.FormatInt(int64(os.O_RDONLY&os.O_RDWR), 2))
	t.Log("spike")
	t.Log(strconv.FormatInt(int64((os.O_RDWR)), 2))
}

func TestFlagHas(t *testing.T) {
	type TestCase struct {
		Desc     string
		FlagBase int
		HasRead  bool
		HasWrite bool
	}
	for _, tc := range []TestCase{
		{
			Desc:     "os.O_RDONLY",
			FlagBase: os.O_RDONLY,
			HasRead:  true,
			HasWrite: false,
		},
		{
			Desc:     "os.O_WRONLY",
			FlagBase: os.O_WRONLY,
			HasRead:  false,
			HasWrite: true,
		},
		{
			Desc:     "os.O_RDWR",
			FlagBase: os.O_RDWR,
			HasRead:  true,
			HasWrite: true,
		},
	} {
		tc := tc
		t.Run(tc.Desc, func(t *testing.T) {
			t.Run("#HasOpenFlagRead", func(t *testing.T) {
				it := assert.MakeIt(t)

				t.Log(tc.FlagBase)
				t.Log(strconv.FormatInt(int64((tc.FlagBase|os.O_APPEND)&os.O_RDONLY), 2))

				assert.Equal(it, tc.HasRead, ffs.HasOpenFlagRead(tc.FlagBase))
				assert.Equal(it, tc.HasRead, ffs.HasOpenFlagRead(tc.FlagBase|os.O_APPEND))
				assert.Equal(it, tc.HasRead, ffs.HasOpenFlagRead(tc.FlagBase|os.O_TRUNC))
				assert.Equal(it, tc.HasRead, ffs.HasOpenFlagRead(tc.FlagBase|os.O_CREATE))
				assert.Equal(it, tc.HasRead, ffs.HasOpenFlagRead(tc.FlagBase|os.O_CREATE|os.O_EXCL))
			})
			t.Run("HasOpenFlagWrite", func(t *testing.T) {
				it := assert.MakeIt(t)
				assert.Equal(it, tc.HasWrite, ffs.HasOpenFlagWrite(tc.FlagBase))
				assert.Equal(it, tc.HasWrite, ffs.HasOpenFlagWrite(tc.FlagBase|os.O_APPEND))
				assert.Equal(it, tc.HasWrite, ffs.HasOpenFlagWrite(tc.FlagBase|os.O_TRUNC))
				assert.Equal(it, tc.HasWrite, ffs.HasOpenFlagWrite(tc.FlagBase|os.O_CREATE))
				assert.Equal(it, tc.HasWrite, ffs.HasOpenFlagWrite(tc.FlagBase|os.O_CREATE|os.O_EXCL))
			})
		})
	}
}
