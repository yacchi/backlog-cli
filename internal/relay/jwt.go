package relay

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OAuthSessionClaims はOAuthセッションのJWTクレーム
type OAuthSessionClaims struct {
	Port    int    `json:"port"`
	State   string `json:"state"`
	Domain  string `json:"domain"`
	Space   string `json:"space"`
	Project string `json:"project,omitempty"`
	jwt.RegisteredClaims
}

const (
	cookieName   = "oauth_session"
	cookieMaxAge = 5 * 60 // 5分
)

// createSessionToken はセッショントークンを作成する
func (s *Server) createSessionToken(port int, state, domain, space, project string) (string, error) {
	server := s.cfg.Server()
	claims := OAuthSessionClaims{
		Port:    port,
		State:   state,
		Domain:  domain,
		Space:   space,
		Project: project,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(server.CookieMaxAge) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.cookieSecret)
}

// parseSessionToken はセッショントークンを検証・パースする
func (s *Server) parseSessionToken(tokenString string) (*OAuthSessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &OAuthSessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.cookieSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*OAuthSessionClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
