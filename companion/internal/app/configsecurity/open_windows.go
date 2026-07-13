//go:build windows

package configsecurity

import "os"

func prepareRoot(string) error {
	return ErrUnsafe
}

func publishNew(string, string) error {
	return ErrUnsafe
}

func syncRoot(string) error {
	return ErrUnsafe
}

func validateRoot(os.FileInfo) error {
	return ErrUnsafe
}

func validateFile(os.FileInfo) error {
	return ErrUnsafe
}
