package filesystem

const (
	modeRead  = 04
	modeWrite = 02
	modeExec  = 01

	modeUserShift  = 6
	modeGroupShift = 3
	modeOtherShift = 0

	ModeUserR   = modeRead << modeUserShift
	ModeUserW   = modeWrite << modeUserShift
	ModeUserX   = modeExec << modeUserShift
	ModeUserRW  = ModeUserR | ModeUserW
	ModeUserRWX = ModeUserRW | ModeUserX

	ModeGroupR   = modeRead << modeGroupShift
	ModeGroupW   = modeWrite << modeGroupShift
	ModeGroupX   = modeExec << modeGroupShift
	ModeGroupRW  = ModeGroupR | ModeGroupW
	ModeGroupRWX = ModeGroupRW | ModeGroupX

	ModeOtherR   = modeRead << modeOtherShift
	ModeOtherW   = modeWrite << modeOtherShift
	ModeOtherX   = modeExec << modeOtherShift
	ModeOtherRW  = ModeOtherR | ModeOtherW
	ModeOtherRWX = ModeOtherRW | ModeOtherX

	ModeAllR   = ModeUserR | ModeGroupR | ModeOtherR
	ModeAllW   = ModeUserW | ModeGroupW | ModeOtherW
	ModeAllX   = ModeUserX | ModeGroupX | ModeOtherX
	ModeAllRW  = ModeAllR | ModeAllW
	ModeAllRWX = ModeAllRW | ModeGroupX
)
