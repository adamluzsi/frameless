package filemode

import "os"

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

// Octal represents Unix file permissions in octal (base-8) notation.
//
// Octal notation is the numeric representation of POSIX file permissions,
// where each digit (0-7) represents a combination of read (4), write (2),
// and execute (1) permissions for user, group, and other respectively.
//
// Format:
//   - 3-digit format: Standard permissions (e.g., 755, 644, 777)
//   - 4-digit format: Special bits + standard permissions (e.g., 4755, 2644, 1777)
//
// Standard 3-digit format:
//   - 2 to left digit: User/owner permissions
//   - 1 to left digit: Group permissions
//   - 0 to left digit: Other/world permissions
//
// Each digit is calculated by summing the permission values:
//   - Read (r):    4
//   - Write (w):   2
//   - Execute (x): 1
//
// Permission value table:
//
//	0 (000): --- (no permissions)
//	1 (001): --x (execute only)
//	2 (010): -w- (write only)
//	3 (011): -wx (write + execute)
//	4 (100): r-- (read only)
//	5 (101): r-x (read + execute)
//	6 (110): rw- (read + write)
//	7 (111): rwx (read + write + execute)
//
// Examples:
//
//	755 = -rwxr-xr-x (user: rwx, group: r-x, other: r-x)
//	644 = -rw-r--r-- (user: rw-, group: r--, other: r--)
//	777 = -rwxrwxrwx (user: rwx, group: rwx, other: rwx)
//	600 = -rw------- (user: rw-, group: ---, other: ---)
//
// Special permission bits (4-digit format):
//   - First digit represents special bits:
//     4000: setuid bit (s in user execute position)
//     2000: setgid bit (s in group execute position)
//     1000: sticky bit (t in other execute position)
//
// Examples with special bits:
//
//	4755 = rwsr-xr-x (setuid + rwxr-xr-x)
//	2755 = rwxr-sr-x (setgid + rwxr-xr-x)
//	1777 = rwxrwxrwt (sticky + rwxrwxrwx)
//
// Conversion from os.FileMode:
//
//	mode.Perm() returns the permission bits as os.FileMode
//	Use Octal(mode.Perm()) to convert to octal representation
//
// Standards:
//   - Defined by POSIX.1 standard
//   - Supported by chmod, stat, and umask commands
//   - Compatible with all Unix-like systems (Linux, macOS, BSD)
type Octal os.FileMode

// Type returns type bits in m (m & [ModeType]).
func (o Octal) Type() Octal {
	return Octal(o.mode().Type())
}

// Perm returns the Unix permission bits in m (m & [ModePerm]).
func (o Octal) Perm() Octal {
	return Octal(o.mode().Perm())
}

func (o Octal) mode() os.FileMode {
	return os.FileMode(o)
}

func (o Octal) User() Permission {
	return Permission{
		Class:   "user",
		Read:    o.hasMode(UserR),
		Write:   o.hasMode(UserW),
		Execute: o.hasMode(UserX),
	}
}

func (o Octal) Group() Permission {
	return Permission{
		Class:   "group",
		Read:    o.hasMode(GroupR),
		Write:   o.hasMode(GroupW),
		Execute: o.hasMode(GroupX),
	}
}

func (o Octal) Other() Permission {
	return Permission{
		Class:   "other",
		Read:    o.hasMode(OtherR),
		Write:   o.hasMode(OtherW),
		Execute: o.hasMode(OtherX),
	}
}

func (o Octal) hasMode(has os.FileMode) bool {
	return os.FileMode(o)&has != 0
}

func Contains(perm, has os.FileMode) bool {
	return Octal(perm).Contains(Octal(has))
}

func (o Octal) Contains(oth Octal) bool {
	if exp := oth.User(); !exp.IsZero() && !o.User().Contains(exp) {
		return false
	}
	if exp := oth.Group(); !exp.IsZero() && !o.Group().Contains(exp) {
		return false
	}
	if exp := oth.Other(); !exp.IsZero() && !o.Other().Contains(exp) {
		return false
	}
	return true
}

// posixSymbolixNotationFileTypeChar returns the ls-style type character for mode.
func (o Octal) posixSymbolixNotationFileTypeChar() string {
	mode := os.FileMode(o.Type())
	switch {
	case mode&os.ModeDir != 0:
		return "d"
	case mode&os.ModeSymlink != 0:
		return "l"
	case mode&os.ModeNamedPipe != 0:
		return "p"
	case mode&os.ModeSocket != 0:
		return "s"
	case mode&os.ModeDevice != 0:
		// Distinguish block vs character device:
		if mode&os.ModeCharDevice != 0 {
			return "c"
		}
		return "b"
	default:
		return "-" // regular file
	}
}

// SymbolicNotation returns a POSIX compliant symbolic notation for a given file mode.
func (o Octal) SymbolicNotation() string {
	var (
		v   string
		usr = o.User()
		grp = o.Group()
		oth = o.Other()
	)
	v += string(o.posixSymbolixNotationFileTypeChar())
	v += o.formatAppend(usr.Read, "r")
	v += o.formatAppend(usr.Write, "w")
	v += o.formatAppend(usr.Execute, "x")
	v += o.formatAppend(grp.Read, "r")
	v += o.formatAppend(grp.Write, "w")
	v += o.formatAppend(grp.Execute, "x")
	v += o.formatAppend(oth.Read, "r")
	v += o.formatAppend(oth.Write, "w")
	v += o.formatAppend(oth.Execute, "x")
	return v
}

func (Octal) formatAppend(has bool, sym string) string {
	if has {
		return sym
	}
	return "-"
}
