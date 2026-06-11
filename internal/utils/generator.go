package utils

import (
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
)

const (
	alphabet   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_"
	base       = uint64(len(alphabet)) // 63
	codeLen    = 10
	upperStart = 0 // A-Z: 0-25
	upperEnd   = 25
	lowerStart = 26 // a-z: 26-51
	lowerEnd   = 51
	digitStart = 52 // 0-9: 52-61
	digitEnd   = 61
	underscore = 62 // _: 62
)

type Generator struct {
	logger *slog.Logger
}

func NewGenerator(logger *slog.Logger) *Generator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Generator{
		logger: logger,
	}
}

func (g *Generator) Generate(originalURL string) string {
	g.logger.Debug("generating short code", "original_url", originalURL)

	hash := sha256.Sum256([]byte(originalURL))
	n := binary.BigEndian.Uint64(hash[:8])

	result := make([]byte, codeLen)

	positions := deterministicShuffle([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, n)

	// A-Z
	result[positions[0]] = alphabet[upperStart+int(n%26)]
	n = n/26 + binary.BigEndian.Uint64(hash[8:16])

	// a-z
	result[positions[1]] = alphabet[lowerStart+int(n%26)]
	n = n/26 + binary.BigEndian.Uint64(hash[16:24])

	// 0-9
	result[positions[2]] = alphabet[digitStart+int(n%10)]
	n = n/10 + binary.BigEndian.Uint64(hash[24:32])

	// _
	result[positions[3]] = alphabet[underscore]

	// заполняем оставшиеся 6 позиций
	remaining := []int{4, 5, 6, 7, 8, 9}
	for i, idx := range remaining {
		pos := positions[idx]
		result[pos] = alphabet[n%base]
		n = n/base + uint64(hash[i%32])
	}

	shortCode := string(result)
	g.logger.Debug("code generated", "original_url", originalURL, "short_code", shortCode)

	return shortCode
}

// deterministicShuffle перемешивает срез детерминированно
func deterministicShuffle(items []int, seed uint64) []int {
	result := make([]int, len(items))
	copy(result, items)

	n := uint64(len(result))
	for i := n - 1; i > 0; i-- {
		j := seed % (i + 1)
		seed = seed/63 + uint64(items[int(j)%32])
		result[i], result[int(j)] = result[int(j)], result[i]
	}
	return result
}

// ValidateShortCode проверяет, что код содержит хотя бы по одному символу каждого типа
func ValidateShortCode(code string) bool {
	if len(code) != codeLen {
		return false
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasUnderscore := false

	for _, c := range code {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '_':
			hasUnderscore = true
		default:
			return false // недопустимый символ
		}
	}

	return hasUpper && hasLower && hasDigit && hasUnderscore
}
