package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var NoTokenError error = errors.New("No authentication token found in request")

type TokenReader interface {
	TokenFromRequest(*http.Request) (string, error)
}

type BearerTokenReader struct {
	store TokenStore
}

func (b *BearerTokenReader) TokenFromRequest(req *http.Request) (string, error) {
	tokenString, err := b.tokenStringFromRequest(req)
	if err != nil {
		return "", err
	}

	token, err := b.store.GetToken(tokenString)
	if err == NoTokenError {
		return "", err
	} else if err != nil {
		return "", fmt.Errorf("error while loading JWT for token %s: %s", tokenString, err)
	}

	return token, nil
}

func (b *BearerTokenReader) tokenStringFromRequest(req *http.Request) (string, error) {
	authHeader := req.Header.Get("Authorization")

	if authHeader != "" {
		elements := strings.SplitN(authHeader, " ", 2)
		if elements[0] != "Bearer" {
			return "", fmt.Errorf("'%s' authorization is not supported", elements[0])
		}

		return elements[1], nil
	}

	cookie, err := req.Cookie("ACCESSTOKEN")
	if err == nil {
		return cookie.Value, nil
	}

	jwtHeader := req.Header.Get("X-JWT")
	if jwtHeader != "" {
		return jwtHeader, nil
	}

	tokenParam := req.URL.Query().Get("access_token")
	if tokenParam != "" {
		return tokenParam, nil
	}

	return "", NoTokenError
}
