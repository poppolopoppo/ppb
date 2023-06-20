package utils

import "testing"

func TestFourCC(t *testing.T) {
	cc0 := MakeFourCC('a', 'b', 'c', 'd')
	cc1 := StringToFourCC(cc0.String())
	if cc0 != cc1 {
		t.Errorf("invalid fourcc: %v != %v", cc0, cc1)
	}
}
