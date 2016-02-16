package auth
import (
	"net/http"
	"strings"
	"fmt"
	"errors"
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

	token, err := b.store.GetTokenFromStore(tokenString)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (b *BearerTokenReader) tokenStringFromRequest(req *http.Request) (Token, error) {
	authHeader := req.Header.Get("Authorization")

	if authHeader != "" {
		elements := strings.SplitN(authHeader, " ", 2)
		if elements[0] != "Bearer" {
			return "", fmt.Errorf("'%s' authorization is not supported", elements[0])
		}

		return Token(elements[1]), nil
	}

	jwtHeader := req.Header.Get("X-JWT")
	if jwtHeader != "" {
		return Token(jwtHeader), nil
	}

	tokenParam := req.URL.Query().Get("access_token")
	if tokenParam != "" {
		return Token(tokenParam), nil
	}

	return "", NoTokenError
}