package localtrust

import "testing"

func TestFindContextTagBytesBounded(t *testing.T) {
	// Self-referential-looking SEQUENCE must not overflow.
	der := []byte{0x30, 0x04, 0x30, 0x02, 0x05, 0x00}
	_, _ = findContextTagBytes(der, 709)
}
