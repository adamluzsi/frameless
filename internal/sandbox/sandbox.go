package sandbox

import (
	"runtime"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/option"
)

type O struct {
	OK     bool
	Panic  bool
	Goexit bool

	PanicValue any
}

func Run(fn func(), opts ...Option) O {
	c := option.ToConfig(opts)
	var done = make(chan struct{})
	var o O
	go func() {
		defer close(done)
		var r any
		defer func() {
			if o.OK {
				return
			}
			if o.Panic {
				if r == nil {
					return
				}
				if _, isNilPanic := r.(*runtime.PanicNilError); isNilPanic {
					return
				}
				o.PanicValue = r
			} else {
				o.Goexit = true
			}
		}()
		func() {
			defer func() {
				if !c.NoRecover {
					r = recover()
				}
			}()
			fn()
			o.OK = true
		}()
		if !o.OK { // panic:true, goexit:false
			o.Panic = true
		}
	}()
	<-done
	return o
}

type Option option.Option[Config]

type Config struct {
	NoRecover bool
}

func (c Config) Configure(t *Config) {
	*t = reflectkit.MergeStruct(*t, c)
}
