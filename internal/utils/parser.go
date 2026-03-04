package utils

import (
	"strings"
	"unicode"
)

// ParseSipUri: "Alice <sip:1001@10.0.0.1:5060;transport=udp>" -> "1001"
func ParseSipUri(uri string) string {
	if uri == "" {
		return "anonymous"
	}

	// 1. Şemayı ve gereksiz kısımları temizle
	s := uri
	if idx := strings.Index(s, "sip:"); idx != -1 {
		s = s[idx+4:]
	} else if idx := strings.Index(s, "sips:"); idx != -1 {
		s = s[idx+5:]
	}

	// 2. Kullanıcı kısmını al ( @ öncesi )
	if idx := strings.Index(s, "@"); idx != -1 {
		s = s[:idx]
	}

	// 3. Parametreleri temizle ( ; öncesi )
	if idx := strings.Index(s, ";"); idx != -1 {
		s = s[:idx]
	}

	// 4. Sadece alphanumeric karakterleri tut (Güvenlik)
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '+' || r == '*' || r == '#' {
			sb.WriteRune(r)
		}
	}

	cleaned := sb.String()
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}

// DetermineDirection: Numara uzunluğuna ve içeriğine göre çağrı yönünü tahmin eder.
func DetermineDirection(caller, callee string) string {
	// Basit kural: Arayan numara uzunsa (905...) ve aranan kısaysa (1001) -> INBOUND
	// Arayan kısaysa (1001) ve aranan uzunsa (905...) -> OUTBOUND
	// İkisi de kısaysa -> INTERNAL

	lenCaller := len(caller)
	lenCallee := len(callee)

	if lenCaller <= 5 && lenCallee <= 5 {
		return "INTERNAL"
	}
	if lenCaller > 5 && lenCallee <= 5 {
		return "INBOUND"
	}
	if lenCaller <= 5 && lenCallee > 5 {
		return "OUTBOUND"
	}

	// Varsayılan
	return "INBOUND"
}
