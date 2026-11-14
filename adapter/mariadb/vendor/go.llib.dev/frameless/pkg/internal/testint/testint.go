package testint

type TB interface {
	Helper()
	Cleanup(func())

	Fail()
	FailNow()
	SkipNow()

	Log(args ...any)
	Logf(format string, args ...any)
}
