package filesystem_test

import (
	"go.llib.dev/frameless/ports/filesystem"
	"io/fs"
	"testing"

	"github.com/adamluzsi/testcase"
)

func TestCanXDoY(t *testing.T) {
	s := testcase.NewSpec(t)

	type TestCase struct {
		Desc string

		FileMode fs.FileMode

		UserR bool
		UserW bool
		UserX bool

		GroupR bool
		GroupW bool
		GroupX bool

		OtherR bool
		OtherW bool
		OtherX bool
	}

	for _, tc := range []TestCase{
		{
			Desc:     "when no-one has permission",
			FileMode: 0000,
		},
		{
			Desc:     "when user permission has read",
			FileMode: 0400,
			UserR:    true,
		},
		{
			Desc:     "when group permission has read",
			FileMode: 0040,
			GroupR:   true,
		},
		{
			Desc:     "when others' permission has read",
			FileMode: 0004,
			OtherR:   true,
		},
		{
			Desc:     "when user permission has execute",
			FileMode: 0100,
			UserX:    true,
		},
		{
			Desc:     "when group permission has execute",
			FileMode: 0010,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has execute",
			FileMode: 0001,
			OtherX:   true,
		},

		{
			Desc:     "when user permission has write-execute",
			FileMode: 0300,
			UserW:    true,
			UserX:    true,
		},
		{
			Desc:     "when group permission has write-execute",
			FileMode: 0030,
			GroupW:   true,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has write-execute",
			FileMode: 0003,
			OtherW:   true,
			OtherX:   true,
		},
		{
			Desc:     "when user permission has write",
			FileMode: 0200,
			UserW:    true,
		},
		{
			Desc:     "when group permission has write",
			FileMode: 0020,
			GroupW:   true,
		},
		{
			Desc:     "when others' permission has write",
			FileMode: 0002,
			OtherW:   true,
		},

		{
			Desc:     "when user permission has read-execute",
			FileMode: 0500,
			UserR:    true,
			UserX:    true,
		},
		{
			Desc:     "when group permission has read-execute",
			FileMode: 0050,
			GroupR:   true,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has read-execute",
			FileMode: 0005,
			OtherR:   true,
			OtherX:   true,
		},
		{
			Desc:     "when user permission has read-write",
			FileMode: 0600,
			UserR:    true,
			UserW:    true,
		},
		{
			Desc:     "when group permission has read-write",
			FileMode: 0060,
			GroupR:   true,
			GroupW:   true,
		},
		{
			Desc:     "when others' permission has read-write",
			FileMode: 0006,
			OtherR:   true,
			OtherW:   true,
		},
		{
			Desc:     "when user permission has read-write-execute",
			FileMode: 0700,
			UserR:    true,
			UserW:    true,
			UserX:    true,
		},
		{
			Desc:     "when group permission has read-write-execute",
			FileMode: 0070,
			GroupR:   true,
			GroupW:   true,
			GroupX:   true,
		},
		{
			Desc:     "when others' permission has read-write-execute",
			FileMode: 0007,
			OtherR:   true,
			OtherW:   true,
			OtherX:   true,
		},
		{
			Desc:     "when user has read-write-execute, group has read-write, others has read",
			FileMode: 0764,
			UserR:    true,
			UserW:    true,
			UserX:    true,
			GroupR:   true,
			GroupW:   true,
			OtherR:   true,
		},
	} {
		tc := tc
		s.Test(tc.Desc, func(t *testcase.T) {

			t.Must.Equal(tc.UserR, tc.FileMode&filesystem.ModeUserR != 0)
			t.Must.Equal(tc.UserW, tc.FileMode&filesystem.ModeUserW != 0)
			t.Must.Equal(tc.UserX, tc.FileMode&filesystem.ModeUserX != 0)

			t.Must.Equal(tc.GroupR, tc.FileMode&filesystem.ModeGroupR != 0)
			t.Must.Equal(tc.GroupW, tc.FileMode&filesystem.ModeGroupW != 0)
			t.Must.Equal(tc.GroupX, tc.FileMode&filesystem.ModeGroupX != 0)

			t.Must.Equal(tc.OtherR, tc.FileMode&filesystem.ModeOtherR != 0)
			t.Must.Equal(tc.OtherW, tc.FileMode&filesystem.ModeOtherW != 0)
			t.Must.Equal(tc.OtherX, tc.FileMode&filesystem.ModeOtherX != 0)

		})
	}
}
