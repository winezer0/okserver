package digest

import "os"

func FileDigest(name string) (string, error) {
	file, err := os.Open(name)
	if os.IsNotExist(err) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	return OpenFileDigest(file)
}
