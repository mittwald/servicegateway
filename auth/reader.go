package auth

import (
	"fmt"
	"github.com/pkg/errors"
	"net/http"
	"strings"
)

var NoTokenError error = errors.New("No authentication token found in request")

type TokenReader interface {
	TokenFromRequest(*http.Request) (*JWTResponse, error)
}

type BearerTokenReader struct {
	store TokenStore
}

func (b *BearerTokenReader) TokenFromRequest(req *http.Request) (*JWTResponse, error) {
	tokenString, err := b.tokenStringFromRequest(req)
	if err == NoTokenError {
		return nil, err
	} else if err != nil {
		return nil, errors.WithStack(err)
	}

	token, err := b.store.GetToken(tokenString)
	if err == NoTokenError {
		return nil, err
	} else if err != nil {
		return nil, fmt.Errorf("error while loading JWT for token %s: %s", tokenString, err)
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

		if len(elements) == 2 {
			return elements[1], nil
		}
	}

	cookie, err := req.Cookie("ACCESSTOKEN")
	if err == nil {
		return cookie.Value, nil
	}

	cookie, err = req.Cookie("access_token")
	if err == nil {
		return cookie.Value, nil
	}

	jwtHeader := req.Header.Get("X-JWT")
	if jwtHeader != "" {
		return jwtHeader, nil
	}

	jwtHeader = req.Header.Get("x-access-token")
	if jwtHeader != "" {
		return jwtHeader, nil
	}

	tokenParam := req.URL.Query().Get("access_token")
	if tokenParam != "" {
		return tokenParam, nil
	}

	return "", NoTokenError
}
