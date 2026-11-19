package digest

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func changeStamp(file *os.File, info os.FileInfo) (string, error) {
	var basic struct {
		CreationTime, LastAccessTime, LastWriteTime, ChangeTime int64
		Attributes                                              uint32
		Padding                                                 uint32
	}
	err := windows.GetFileInformationByHandleEx(windows.Handle(file.Fd()), windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic)))
	return fmt.Sprint(basic.ChangeTime), err
}
