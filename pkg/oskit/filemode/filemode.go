package filemode

const (
	read    = 4
	write   = 2
	execute = 1

	shiftUser  = 6
	shiftGroup = 3
	shiftOther = 0

	UserR   = read << shiftUser
	UserW   = write << shiftUser
	UserX   = execute << shiftUser
	UserRW  = UserR | UserW
	UserRWX = UserRW | UserX

	GroupR   = read << shiftGroup
	GroupW   = write << shiftGroup
	GroupX   = execute << shiftGroup
	GroupRW  = GroupR | GroupW
	GroupRWX = GroupRW | GroupX

	OtherR   = read << shiftOther
	OtherW   = write << shiftOther
	OtherX   = execute << shiftOther
	OtherRW  = OtherR | OtherW
	OtherRWX = OtherRW | OtherX

	AllR   = UserR | GroupR | OtherR
	AllW   = UserW | GroupW | OtherW
	AllX   = UserX | GroupX | OtherX
	AllRW  = AllR | AllW
	AllRWX = AllRW | AllX
)
