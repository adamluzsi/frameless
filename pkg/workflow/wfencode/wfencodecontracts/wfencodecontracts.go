package wfencodecontracts

import (
	"github.com/adamluzsi/frameless/internal/suites"
	wf "github.com/adamluzsi/frameless/pkg/workflow"
	"github.com/adamluzsi/testcase"
)

type MarshalFunc func(v any) ([]byte, error)

type UnmarshalFunc func(data []byte, v any) error

func Encoding(marshal MarshalFunc, unmarshal UnmarshalFunc) suites.Suite {
	s := testcase.NewSpec(nil, testcase.AsSuite("workflow encoding"))

	s.Context("Task", func(s *testcase.Spec) {
		cases := map[string]wf.Task{
			"Template": wf.Template(".X != .Y"),
			"If": wf.If{
				Cond: wf.Template(`.X == 42`),
				Then: wf.Template(`.Foo`),
				Else: wf.Template(`.Bar`),
			},
			"Sequence": wf.MakeSequence(
				wf.Template(`.Foo`),
				wf.Template(`.Bar`),
				wf.Template(`.Baz`),
			),
			"Concurrence": wf.MakeConcurrence(
				wf.Template(`.Foo`),
				wf.Template(`.Bar`),
				wf.Template(`.Baz`),
			),
			"UseParticipant": wf.UseParticipant{
				ID:   "the-id-of-the-participant",
				Args: []wf.Value{wf.ConstValue{Value: "42"}, wf.RefValue{Key: "x"}},
				Out:  []wf.VarKey{"x"},
			},
			"While": wf.While{
				Cond:  wf.Template(`.X < 42`),
				Block: wf.Template(`set X .X+1`),
			},
			"Goto": wf.Goto{
				LabelID: wf.LabelID("the-label-id"),
			},
			"Label": wf.Label{
				ID:   "the-label-id",
				Task: wf.Template(`set X .X+1`),
			},
		}
		testcase.TableTest(s, cases, func(t *testcase.T, exp wf.Task) {
			data, err := marshal(exp)
			t.Must.NoError(err)
			var got wf.TaskID
			t.Must.NoError(unmarshal(data, &got))
			t.Must.Equal(exp, got)
		})
	})

	return s.AsSuite()
}
