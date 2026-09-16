package storage

import (
	"os"
)

const PrivatePerm os.FileMode = 0o600

func WriteSecret(path string, data []byte) error {
	return AtomicWriteFile(path, data, PrivatePerm)
}

func EnforceSecretPerm(path string) error {
	return os.Chmod(path, PrivatePerm)
}
