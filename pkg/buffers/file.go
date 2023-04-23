package buffers

//
//import (
//	"github.com/adamluzsi/frameless/pkg/errorutil"
//	"github.com/adamluzsi/frameless/pkg/zeroutil"
//	"io"
//	"os"
//)
//
//type Repeatable struct {
//	// KeepInMemorySize is the byte size till the buffer is willing to keep the memory
//	// before it starts to write things into the filesystem.
//	//
//	// Zero value interpreted as 10MB.
//	KeepInMemorySize int
//
//	len     int
//	buf     Buffer
//	tmpFile *os.File
//}
//
//func (r *Repeatable) Repeatable() error {
//
//}
//
//func (r *Repeatable) Read(p []byte) (n int, err error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r *Repeatable) Write(p []byte) (n int, err error) {
//	defer func() { r.len += n }()
//
//	if r.len+len(p) <= r.getMemSize() {
//		return r.buf.Write(p)
//	}
//
//	r.getTmpFile()
//
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r *Repeatable) Close() error {
//	if r.tmpFile == nil {
//		return nil
//	}
//	if err := r.tmpFile.Close(); err != nil {
//		return err
//	}
//	r.tmpFile = nil
//}
//
//func (r *Repeatable) getMemSize() int {
//	const defaultMemSize = 10 * 1024 * 1024 // 10MB
//	return zeroutil.Init(&r.KeepInMemorySize, func() int {
//		return defaultMemSize
//	})
//}
//
//func (r *Repeatable) getTmpFile() (*os.File, error) {
//	if r.tmpFile == nil {
//		tmpFile, err := os.CreateTemp(os.TempDir(), "buffers-repeatable-*****")
//		if err != nil {
//			return nil, err
//		}
//		// copy the current buffer content
//		if _, err := io.Copy(tmpFile, &r.buf); err != nil {
//			return nil, errorutil.Merge(
//				err,
//				tmpFile.Close(),
//				os.Remove(tmpFile.Name()),
//			)
//		}
//		r.tmpFile = tmpFile
//	}
//	return r.tmpFile, nil
//}
