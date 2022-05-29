package filesystems

import (
	"os"
)

func flagHasRead(flag int) bool {
	if flag&os.O_RDWR != 0 {
		return true
	}
	if flag&os.O_WRONLY != 0 {
		return false
	}
	return true
}

func flagHasWrite(flag int) bool {
	return flag&os.O_RDWR != 0 || flag&os.O_WRONLY != 0
}
