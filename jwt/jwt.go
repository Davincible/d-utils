package jwt

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
)

var DefaultJWTDuration = 6 * time.Hour

// JwtUser is the JWT user.
type JwtUser struct {
	jwt.StandardClaims

	UserID    int64 `json:"user_id"`
	ChatID    int64 `json:"chat_id"`
	SessionID int64 `json:"session_id"`
	IsAdmin   bool  `json:"is_admin"`
}

func extractToken(hdr string) (string, error) {
	if hdr == "" {
		return "", errors.New("no authorization header")
	}

	th := strings.Split(hdr, " ")
	if len(th) != 2 {
		return "", errors.New("incomplete authorization header")
	}

	if strings.ToLower(th[0]) == "bearer" {
		return th[1], nil
	} else if strings.ToLower(th[0]) == "token" {
		return th[1], nil
	}

	return "", errors.New("unknow token format")
}

// DecodeJWTToken decodes the JWT token from the cookie and returns the user.
func DecodeJWTToken(r *http.Request) (*JwtUser, error) {
	bearer := r.Header.Get("Authorization")
	if bearer == "" {
		h := r.Header.Get("Sec-Websocket-Protocol")

		if splits := strings.Split(h, " "); len(splits) > 1 {
			bearer = "bearer " + strings.TrimSpace(splits[1])
		}
	}

	token, err := extractToken(bearer)
	if err != nil {
		return nil, err
	}

	user, err := DecodeJwtToken(token)
	if err != nil {
		return nil, err
	}

	// if user.Issuer != os.Getenv("WEB_APP_URL") {
	// 	return nil, errors.New("invalid issuer")
	// }
	//
	// if user.Subject != os.Getenv("WEB_APP_URL") {
	// 	return nil, errors.New("invalid issuer")
	// }

	return user, nil
}

// DecodeJwtToken decodes the JWT token and returns the user.
func DecodeJwtToken(tokenString string) (*JwtUser, error) {
	if len(tokenString) == 0 {
		return nil, errors.New("empty JWT token provided")
	}

	key := os.Getenv("JWT_SECRET_KEY")
	if len(key) == 0 {
		return nil, errors.New("JWT_SECRET_KEY is not set")
	}

	var jwtUser JwtUser

	token, err := jwt.ParseWithClaims(tokenString, &jwtUser, func(token *jwt.Token) (any, error) {
		return []byte(key), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse JWT token claims: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return &jwtUser, nil
}

// SignJWTToken creates a JWT token for the user.
func SignJWTToken(userID, chatID int64, isAdmin bool) (string, error) {
	return SignJWTTokenWithDuration(userID, chatID, isAdmin, DefaultJWTDuration)
}

func SignJWTTokenWithDuration(userID, chatID int64, isAdmin bool, duration time.Duration) (string, error) {
	key := os.Getenv("JWT_SECRET_KEY")
	if len(key) == 0 {
		return "", errors.New("JWT_SECRET_KEY is not set")
	}

	t := time.Now().Unix()

	tokenStr := jwt.NewWithClaims(jwt.SigningMethodHS256, JwtUser{
		StandardClaims: jwt.StandardClaims{
			IssuedAt:  t,
			NotBefore: t,
			Issuer:    os.Getenv("WEB_APP_URL"),
			Subject:   os.Getenv("WEB_APP_URL"),
			ExpiresAt: time.Now().Add(duration).Unix(),
		},
		UserID:  userID,
		ChatID:  chatID,
		IsAdmin: isAdmin,
	})

	token, err := tokenStr.SignedString([]byte(key))
	if err != nil {
		return "", fmt.Errorf("sign JWT token: %w", err)
	}

	return token, nil
}
