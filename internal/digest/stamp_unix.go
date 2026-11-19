//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package digest

import (
	"fmt"
	"os"
	"reflect"
)

func changeStamp(file *os.File, info os.FileInfo) (string, error) {
	value := reflect.Indirect(reflect.ValueOf(info.Sys()))
	for _, name := range []string{"Ctim", "Ctimespec"} {
		field := value.FieldByName(name)
		if field.IsValid() {
			return fmt.Sprint(field.Interface()), nil
		}
	}
	return "", fmt.Errorf("filesystem change timestamp unavailable")
}
