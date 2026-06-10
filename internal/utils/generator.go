package utils

import (
	"crypto/sha256"
	"encoding/binary"
)

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_"
const base = uint64(len(alphabet)) // 63

type Generator struct {}

func NewGenerator() *Generator {
	return &Generator{}
}

// Generate создаёт 10-символьный уникальный код из URL.
// Детерминирован: один и тот же URL всегда даёт один и тот же код.
func (g *Generator) Generate(originalURL string) string {
	// 1. Криптографический хеш → равномерное распределение бит
	hash := sha256.Sum256([]byte(originalURL))

	// 2. Берём первые 8 байт (64 бита). 
	// 63^10 ≈ 9.8×10¹⁸ < 2^64, поэтому 64 бит достаточно для 10 символов без потери энтропии.
	n := binary.BigEndian.Uint64(hash[:8])

	// 3. Конвертируем число в систему счисления с основанием 63
	var result [10]byte
	for i := 9; i >= 0; i-- {
		result[i] = alphabet[n%base]
		n /= base
	}

	return string(result[:])
}