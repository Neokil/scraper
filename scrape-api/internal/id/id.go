package id

import (
	"crypto/rand"
	"encoding/hex"
)

func New(prefix string) string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(data[:])
}
