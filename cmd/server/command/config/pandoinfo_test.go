package config

import (
	"testing"
)

func TestGetPandoInfoFromKenLabs(t *testing.T) {
	info, err := GetPandoInfo()
	if err != nil {
		t.Error(err)
	}
	addrInfo, err := info.AddrInfo()
	if err != nil {
		t.Error(err)
	}
	t.Log(addrInfo.String())
}
