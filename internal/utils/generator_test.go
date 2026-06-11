package utils

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerator_Generate(t *testing.T) {
	g := NewGenerator(nil)

	t.Run("returns exactly 10 characters", func(t *testing.T) {
		code := g.Generate("https://example.com")
		require.Len(t, code, 10)
	})

	t.Run("uses only allowed alphabet [A-Za-z0-9_]", func(t *testing.T) {
		valid := regexp.MustCompile(`^[A-Za-z0-9_]{10}$`)
		code := g.Generate("https://example.com")
		assert.True(t, valid.MatchString(code), "code %q contains invalid characters", code)
	})

	t.Run("is deterministic: same URL produces same code", func(t *testing.T) {
		url := "https://example.com/path/to/resource"
		code1 := g.Generate(url)
		code2 := g.Generate(url)
		assert.Equal(t, code1, code2, "generator must be deterministic")
	})

	t.Run("different URLs produce different codes", func(t *testing.T) {
		urls := []string{
			"https://example.com",
			"https://example.org",
			"https://example.com/path",
			"https://example.com/path?query=1",
		}
		codes := make(map[string]string)
		for _, url := range urls {
			code := g.Generate(url)
			if existing, ok := codes[code]; ok {
				t.Errorf("collision: URLs %q and %q both produced code %q", url, existing, code)
			}
			codes[code] = url
		}
	})

	t.Run("contains at least one lowercase letter", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := g.Generate("https://example.com/" + string(rune(i)))
			hasLower := false
			for _, c := range code {
				if c >= 'a' && c <= 'z' {
					hasLower = true
					break
				}
			}
			assert.True(t, hasLower, "code %q must contain at least one lowercase letter", code)
		}
	})

	t.Run("contains at least one uppercase letter", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := g.Generate("https://example.com/" + string(rune(i)))
			hasUpper := false
			for _, c := range code {
				if c >= 'A' && c <= 'Z' {
					hasUpper = true
					break
				}
			}
			assert.True(t, hasUpper, "code %q must contain at least one uppercase letter", code)
		}
	})

	t.Run("contains at least one digit", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := g.Generate("https://example.com/" + string(rune(i)))
			hasDigit := false
			for _, c := range code {
				if c >= '0' && c <= '9' {
					hasDigit = true
					break
				}
			}
			assert.True(t, hasDigit, "code %q must contain at least one digit", code)
		}
	})

	t.Run("contains at least one underscore", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := g.Generate("https://example.com/" + string(rune(i)))
			hasUnderscore := false
			for _, c := range code {
				if c == '_' {
					hasUnderscore = true
					break
				}
			}
			assert.True(t, hasUnderscore, "code %q must contain at least one underscore", code)
		}
	})

	t.Run("passes ValidateShortCode", func(t *testing.T) {
		urls := []string{
			"https://example.com",
			"http://localhost:8080/api",
			"https://site.ru/path/to/page?foo=bar&baz=qux",
		}
		for _, url := range urls {
			code := g.Generate(url)
			assert.True(t, ValidateShortCode(code), "code %q for URL %q failed validation", code, url)
		}
	})
}

func TestValidateShortCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		// Валидные коды: ровно 10 символов + присутствуют все 4 типа символов
		{"valid mixed case", "Ab3_xK9mLp", true},
		{"valid all types", "Aa0_BbCcDd", true},           // 10 chars: A,a,0,_,B,b,C,c,D,d
		{"valid underscore at end", "Aa1_bcd_ef", true},   // 10 chars: A,a,1,_,b,c,d,_,e,f
		{"valid underscore at start", "_Aa1bcdefg", true}, // 10 chars: _,A,a,1,b,c,d,e,f,g
		{"valid minimal types", "Aa0_______", true},       // 10 chars: минимально 1 каждого типа + 6 подчеркиваний

		// Неверная длина
		{"too short", "Ab3_xK9mL", false},  // 9 символов
		{"too long", "Ab3_xK9mLpX", false}, // 11 символов

		// Недостающие типы символов
		{"missing lowercase", "AB3_XK9MLP", false},  // нет a-z
		{"missing uppercase", "ab3_xk9mlp", false},  // нет A-Z
		{"missing digit", "Abc_xKpMnQ", false},      // нет 0-9
		{"missing underscore", "Ab3xK9mLpQ", false}, // нет _

		// Недопустимые символы
		{"has hyphen", "Ab3-xK9mLp", false},
		{"has dot", "Ab3.xK9mLp", false},
		{"has space", "Ab3 xK9mLp", false},
		{"has special char", "Ab3!xK9mLp", false},

		// Пустой и другие
		{"empty", "", false},
		{"cyrillic", "Абвгдежзик", false},

		// Только один тип символов
		{"only underscores", "__________", false},
		{"only digits", "1234567890", false},
		{"only lowercase", "abcdefghij", false},
		{"only uppercase", "ABCDEFGHIJ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ValidateShortCode(tt.code))
		})
	}
}

func TestDeterministicShuffle(t *testing.T) {
	t.Run("same seed produces same permutation", func(t *testing.T) {
		items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		seed := uint64(123456789)

		result1 := deterministicShuffle(items, seed)
		result2 := deterministicShuffle(items, seed)

		assert.Equal(t, result1, result2, "shuffle must be deterministic")
	})

	t.Run("different seeds produce different permutations", func(t *testing.T) {
		items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		result1 := deterministicShuffle(items, 100)
		result2 := deterministicShuffle(items, 200)
		assert.NotEqual(t, result1, result2, "different seeds should produce different results")
	})

	t.Run("output contains same elements as input", func(t *testing.T) {
		items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		result := deterministicShuffle(items, 42)

		// Check that result is a permutation of items
		counts := make(map[int]int)
		for _, v := range items {
			counts[v]++
		}
		for _, v := range result {
			counts[v]--
			if counts[v] < 0 {
				t.Errorf("element %d appears more times in result than in input", v)
			}
		}
		for v, c := range counts {
			if c != 0 {
				t.Errorf("element %d count mismatch: %d", v, c)
			}
		}
	})

	t.Run("output length equals input length", func(t *testing.T) {
		items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		result := deterministicShuffle(items, 42)
		assert.Len(t, result, len(items))
	})
}

func TestGenerator_EdgeCases(t *testing.T) {
	g := NewGenerator(nil)

	t.Run("empty URL", func(t *testing.T) {
		code := g.Generate("")
		require.Len(t, code, 10)
		assert.True(t, ValidateShortCode(code))
	})

	t.Run("very long URL", func(t *testing.T) {
		longURL := "https://example.com/" + string(make([]byte, 10000))
		code := g.Generate(longURL)
		require.Len(t, code, 10)
		assert.True(t, ValidateShortCode(code))
	})

	t.Run("URL with unicode", func(t *testing.T) {
		code := g.Generate("https://example.com/привет/мир")
		require.Len(t, code, 10)
		assert.True(t, ValidateShortCode(code))
	})

	t.Run("URL with special characters", func(t *testing.T) {
		code := g.Generate("https://example.com/path?foo=bar&baz=qux#fragment")
		require.Len(t, code, 10)
		assert.True(t, ValidateShortCode(code))
	})
}

func BenchmarkGenerator_Generate(b *testing.B) {
	g := NewGenerator(nil)
	url := "https://example.com/path/to/resource?query=param"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Generate(url)
	}
}

func BenchmarkValidateShortCode(b *testing.B) {
	code := "Ab3_xK9mLp"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateShortCode(code)
	}
}
