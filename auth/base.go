package auth

import (
	"net/http"
	"mittwald.de/servicegateway/config"
	"errors"
	"fmt"
	"io/ioutil"
	"time"
	"sync"
	jwt "github.com/dgrijalva/jwt-go"
)

var NoTokenError error = errors.New("no authentication token present")

type AuthDecorator interface {
	DecorateHandler(http.HandlerFunc) http.HandlerFunc
}

type AuthenticationHandler struct {
	config *config.GlobalAuth
	getToken func(*http.Request) (string, error)
	cacheTtl            time.Duration
	cachedKey           string
	cachedKeyExpiration time.Time
	cachedKeyLock       sync.Mutex
}

func NewAuthenticationHandler(cfg *config.GlobalAuth) (*AuthenticationHandler, error) {
	var tokenReader func(*http.Request) (string, error)

	switch cfg.StorageConfig.Mode {
	case "cookie":
		tokenReader = func(req *http.Request) (string, error) {
			cookie, err := req.Cookie(cfg.StorageConfig.Name)
			if err != nil {
				if err == http.ErrNoCookie {
					return "", NoTokenError
				} else {
					return "", err
				}
			}

			return cookie.Value, nil
		}
	case "header":
		tokenReader = func(req *http.Request) (string, error) {
			header, ok := req.Header[cfg.StorageConfig.Name]
			if ok {
				return header[0], nil
			} else {
				return "", NoTokenError
			}
		}
	default:
		return nil, errors.New(fmt.Sprintf("unsupported token storage mode: '%s'", cfg.StorageConfig.Mode))
	}

	cacheTtl, err := time.ParseDuration(cfg.KeyCacheTtl)
	if err != nil {
		return nil, err
	}

	handler := AuthenticationHandler{
		config: cfg,
		getToken: tokenReader,
		cacheTtl: cacheTtl,
	}

	return &handler, nil
}

func (h *AuthenticationHandler) GetVerificationKey() (string, error) {
	if h.config.VerificationKey != "" {
		return h.config.VerificationKey, nil
	}

	if h.cachedKey != "" && h.cachedKeyExpiration.After(time.Now()) {
		return h.cachedKey, nil
	} else {
		h.cachedKeyLock.Lock()
		defer h.cachedKeyLock.Unlock()

		if h.cachedKey != "" && h.cachedKeyExpiration.After(time.Now()) {
			return h.cachedKey, nil
		}

		resp, err := http.Get(h.config.VerificationKeyUrl)
		if err != nil {
			return "", errors.New(fmt.Sprintf("Could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err))
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", errors.New(fmt.Sprintf("Could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err))
		}

		h.cachedKey = string(body)
		h.cachedKeyExpiration = time.Now().Add(h.cacheTtl)

		return h.cachedKey, nil
	}
}

func (h *AuthenticationHandler) IsAuthenticated(req *http.Request) (bool, error) {
	token, err := h.getToken(req)
	if err != nil {
		if err == NoTokenError {
			return false, nil
		} else {
			return false, err
		}
	}

	key, err := h.GetVerificationKey()
	if err != nil {
		return false, err
	}

	var keyFunc jwt.Keyfunc = func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Header["alg"])
		}
		return key, nil
	}

	dec, err := jwt.Parse(token, keyFunc)
	if err == nil && dec.Valid {
		return true, nil
	}

	return false, nil
}

func NewAuthHandler(authConfig *config.GlobalAuth) (AuthDecorator, error) {
	authHandler, err := NewAuthenticationHandler(authConfig)
	if err != nil {
		return nil, err
	}

	switch authConfig.Mode {
	case "graphical":
		return &GraphicalAuthDecorator{
			authHandler,
			authConfig,
		}, nil
	case "rest":
		return &RestAuthDecorator{
			authHandler,
		}, nil
	}
	return nil, errors.New(fmt.Sprintf("Unsupported authentication mode: '%s'", authConfig.Mode))
}
