package model

import "encoding/hex"

func (p Protocol) IsHelperProtocol() bool {
	return p == TUIC || p == MTProto || p == AmneziaWG
}

func ValidMtprotoAdTag(tag string) bool {
	if tag == "" {
		return true
	}
	if len(tag) != 32 {
		return false
	}
	_, err := hex.DecodeString(tag)
	return err == nil
}
