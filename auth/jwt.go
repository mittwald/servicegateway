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

type JWTVerifier struct {
	config              *config.GlobalAuth
	cacheTTL            time.Duration
	cachedKey           []byte
	cachedKeyExpiration time.Time
	cachedKeyLock       sync.Mutex
}

func NewJwtVerifier(cfg *config.GlobalAuth) (*JWTVerifier, error) {
	cacheTTL, err := time.ParseDuration(cfg.KeyCacheTtl)
	if err != nil {
		return nil, err
	}

	return &JWTVerifier{
		config:   cfg,
		cacheTTL: cacheTTL,
	}, nil
}

func (h *JWTVerifier) GetVerificationKey() ([]byte, error) {
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
		return nil, fmt.Errorf("Could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err)
	}

	h.cachedKey = body
	h.cachedKeyExpiration = time.Now().Add(h.cacheTTL)

	return h.cachedKey, nil
}

func (h *JWTVerifier) VerifyToken(token string) (bool, map[string]interface{}, error) {
	key, err := h.GetVerificationKey()
	if err != nil {
		return false, nil, err
	}

	var keyFunc jwt.Keyfunc = func(decodedToken *jwt.Token) (interface{}, error) {
		if _, ok := decodedToken.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", decodedToken.Header["alg"])
		}
		return key, nil
	}

	dec, err := jwt.Parse(token, keyFunc)
	if err == nil && dec.Valid {
		return true, dec.Claims, nil
	}

	return false, nil, err
}
