package auth

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/mittwald/servicegateway/config"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type JwtVerifier struct {
	config              *config.GlobalAuth
	cacheTtl            time.Duration
	cachedKey           []byte
	cachedKeyExpiration time.Time
	cachedKeyLock       sync.Mutex
}

func NewJwtVerifier(cfg *config.GlobalAuth) (*JwtVerifier, error) {
	cacheTtl, err := time.ParseDuration(cfg.KeyCacheTtl)
	if err != nil {
		return nil, err
	}

	return &JwtVerifier{
		config:   cfg,
		cacheTtl: cacheTtl,
	}, nil
}

func (h *JwtVerifier) GetVerificationKey() ([]byte, error) {
	if h.config.VerificationKey != nil && len(h.config.VerificationKey) > 0 {
		return h.config.VerificationKey, nil
	}

	if h.cachedKey != nil && h.cachedKeyExpiration.After(time.Now()) {
		return h.cachedKey, nil
	}

	h.cachedKeyLock.Lock()
	defer h.cachedKeyLock.Unlock()

	if h.cachedKey != nil && h.cachedKeyExpiration.After(time.Now()) {
		return h.cachedKey, nil
	}

	resp, err := http.Get(h.config.VerificationKeyUrl)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err)
	}

	h.cachedKey = body
	h.cachedKeyExpiration = time.Now().Add(h.cacheTtl)

	return h.cachedKey, nil
}

func (h *JwtVerifier) VerifyToken(token string) (bool, *jwt.StandardClaims, jwt.MapClaims, error) {
	keyPEM, err := h.GetVerificationKey()
	if err != nil {
		return false, nil, nil, fmt.Errorf("error while getting verification key. Err: '%+v'", err)
	}

	t, err := jwt.Parse(token, func(decodedToken *jwt.Token) (interface{}, error) {
		if _, ok := decodedToken.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", decodedToken.Header["alg"])
		}
		return jwt.ParseRSAPublicKeyFromPEM(keyPEM)
	})
	if err != nil {
		return false, nil, nil, fmt.Errorf("error while parsing token with claims. Err: '%+v'", err)
	}

	standardClaims, ok := t.Claims.(*jwt.StandardClaims)
	if !ok {
		return false, nil, nil, fmt.Errorf("error while type-casting claims. Err: '%+v'", err)
	}

	mapClaims, ok := t.Claims.(jwt.MapClaims)
	if !ok {
		return false, nil, nil, fmt.Errorf("error while type-casting claims. Err: '%+v'", err)
	}

	return true, standardClaims, mapClaims, err
}
