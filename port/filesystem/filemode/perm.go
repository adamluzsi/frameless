package filemode

type Permission struct {
	Class   string `enum:"user,group,other"`
	Read    bool
	Write   bool
	Execute bool
}

func (p Permission) IsZero() bool {
	return p == (Permission{})
}

func (p Permission) Contains(o Permission) bool {
	if o.IsZero() {
		return true
	}
	if p.IsZero() {
		return false
	}
	if p.Class != o.Class {
		return false
	}
	if o.Read && !p.Read {
		return false
	}
	if o.Write && !p.Write {
		return false
	}
	if o.Execute && !p.Execute {
		return false
	}
	return true
}

var classShiftMapping = map[string]Octal{
	"user":  shiftUser,
	"group": shiftGroup,
	"other": shiftOther,
}

func (p Permission) Octal() Octal {
	var o Octal
	if p.Read {
		o |= read
	}
	if p.Write {
		o |= write
	}
	if p.Execute {
		o |= execute
	}
	if shift, ok := classShiftMapping[p.Class]; ok {
		o <<= shift
	}
	return o
}
