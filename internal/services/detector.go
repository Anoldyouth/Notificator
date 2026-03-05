package services

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"

	"notificator/internal/domain"
)

type PrefixAddressDetector struct{}

const (
	bech32Const  = 1
	bech32mConst = 0x2bc830a3
)

var (
	base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	base58Map      = makeBase58Map()

	bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
	bech32Map     = makeBech32Map()
)

func (d *PrefixAddressDetector) Detect(address string) (domain.Currency, error) {
	a := strings.TrimSpace(address)
	if isValidBTCAddress(a) {
		return domain.BTC, nil
	}

	if isValidTRXAddress(a) {
		return domain.TRX, nil
	}

	return "", fmt.Errorf("unknown address format: %s", address)
}

func isValidBTCAddress(address string) bool {
	return isValidBTCBase58Address(address) || isValidBTCBech32Address(address)
}

func isValidTRXAddress(address string) bool {
	if !strings.HasPrefix(address, "T") {
		return false
	}

	decoded, err := decodeBase58(address)
	if err != nil {
		return false
	}

	if len(decoded) != 25 {
		return false
	}

	payload := decoded[:len(decoded)-4]
	checksum := decoded[21:]

	if payload[0] != 0x41 {
		return false
	}

	expected := doubleSHA256(payload)
	return bytes.Equal(checksum, expected[:4])
}

func doubleSHA256(data []byte) [32]byte {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

func isValidBTCBase58Address(address string) bool {
	decoded, err := decodeBase58(address)
	if err != nil || len(decoded) != 25 {
		return false
	}

	payload := decoded[:len(decoded)-4]
	checksum := decoded[len(decoded)-4:]
	version := payload[0]

	if version != 0x00 && version != 0x05 {
		return false
	}

	expected := doubleSHA256(payload)
	return bytes.Equal(checksum, expected[:4])
}

func isValidBTCBech32Address(address string) bool {
	if len(address) < 8 || len(address) > 90 || isMixedCase(address) {
		return false
	}

	a := strings.ToLower(address)
	sep := strings.LastIndexByte(a, '1')
	if sep <= 0 || sep+7 > len(a) {
		return false
	}

	hrp := a[:sep]
	if hrp != "bc" {
		return false
	}

	dataPart := a[sep+1:]
	data := make([]byte, len(dataPart))
	for i := range dataPart {
		v, ok := bech32Map[dataPart[i]]
		if !ok {
			return false
		}
		data[i] = v
	}

	if len(data) < 7 {
		return false
	}

	polymod := bech32Polymod(append(bech32HrpExpand(hrp), data...))
	spec := 0
	switch polymod {
	case bech32Const:
		spec = bech32Const
	case bech32mConst:
		spec = bech32mConst
	default:
		return false
	}

	witnessVersion := int(data[0])
	if witnessVersion < 0 || witnessVersion > 16 {
		return false
	}

	program, ok := convertBits(data[1:len(data)-6], 5, 8, false)
	if !ok {
		return false
	}

	if len(program) < 2 || len(program) > 40 {
		return false
	}

	if witnessVersion == 0 {
		if spec != bech32Const {
			return false
		}
		return len(program) == 20 || len(program) == 32
	}

	return spec == bech32mConst
}

func decodeBase58(input string) ([]byte, error) {
	if input == "" {
		return nil, fmt.Errorf("base58: empty input")
	}

	result := big.NewInt(0)
	base := big.NewInt(58)

	for i := 0; i < len(input); i++ {
		v, ok := base58Map[input[i]]
		if !ok {
			return nil, fmt.Errorf("base58: invalid character")
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(v)))
	}

	decoded := result.Bytes()
	leadingOnes := 0
	for leadingOnes < len(input) && input[leadingOnes] == '1' {
		leadingOnes++
	}

	out := make([]byte, leadingOnes+len(decoded))
	copy(out[leadingOnes:], decoded)
	return out, nil
}

func makeBase58Map() map[byte]int {
	m := make(map[byte]int, len(base58Alphabet))
	for i := 0; i < len(base58Alphabet); i++ {
		m[base58Alphabet[i]] = i
	}
	return m
}

func makeBech32Map() map[byte]byte {
	m := make(map[byte]byte, len(bech32Charset))
	for i := 0; i < len(bech32Charset); i++ {
		m[bech32Charset[i]] = byte(i)
	}
	return m
}

func isMixedCase(s string) bool {
	hasLower := false
	hasUpper := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			hasLower = true
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if hasLower && hasUpper {
			return true
		}
	}
	return false
}

func bech32HrpExpand(hrp string) []byte {
	out := make([]byte, 0, len(hrp)*2+1)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]&31)
	}
	return out
}

func bech32Polymod(values []byte) int {
	chk := 1
	generator := [5]int{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ int(v)
		for i := 0; i < 5; i++ {
			if ((top >> i) & 1) != 0 {
				chk ^= generator[i]
			}
		}
	}
	return chk
}

func convertBits(data []byte, fromBits, toBits int, pad bool) ([]byte, bool) {
	acc := 0
	bits := 0
	maxv := (1 << toBits) - 1
	maxAcc := (1 << (fromBits + toBits - 1)) - 1
	out := make([]byte, 0, len(data)*fromBits/toBits+1)

	for _, value := range data {
		v := int(value)
		if v < 0 || (v>>fromBits) != 0 {
			return nil, false
		}
		acc = ((acc << fromBits) | v) & maxAcc
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			out = append(out, byte((acc>>bits)&maxv))
		}
	}

	if pad {
		if bits > 0 {
			out = append(out, byte((acc<<(toBits-bits))&maxv))
		}
	} else if bits >= fromBits || ((acc<<(toBits-bits))&maxv) != 0 {
		return nil, false
	}

	return out, true
}
