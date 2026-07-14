//go:build !windows

package hostmaintenance

func scheduleMaintenance(string, string, string) error { return ErrUnsupported }

func WaitForParent(int) error { return ErrUnsupported }

func CleanupSelf() error { return ErrUnsupported }
