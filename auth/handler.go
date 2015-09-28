package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/garyburd/redigo/redis"
	"github.com/op/go-logging"
	"io/ioutil"
	"github.com/mittwald/servicegateway/config"
	"net/http"
	"sync"
	"time"
)

type AuthenticationHandler struct {
	config              *config.GlobalAuth
	storage             TokenStorage
	cacheTtl            time.Duration
	cachedKey           []byte
	cachedKeyExpiration time.Time
	cachedKeyLock       sync.Mutex
	httpClient          *http.Client
	logger              *logging.Logger
}

func NewAuthenticationHandler(cfg *config.GlobalAuth, redisPool *redis.Pool, logger *logging.Logger) (*AuthenticationHandler, error) {
	var storage TokenStorage

	switch cfg.StorageConfig.Mode {
	case "cookie":
		storage = &CookieTokenStorage{cfg: &cfg.StorageConfig, log: logger}
	case "header":
		storage = &HeaderTokenStorage{cfg: &cfg.StorageConfig, log: logger}
	case "session":
		storage = &SessionTokenStorage{cfg: &cfg.StorageConfig, log: logger, redisPool: redisPool}
	default:
		return nil, fmt.Errorf("unsupported token storage mode: '%s'", cfg.StorageConfig.Mode)
	}

	cacheTtl, err := time.ParseDuration(cfg.KeyCacheTtl)
	if err != nil {
		return nil, err
	}

	handler := AuthenticationHandler{
		config:     cfg,
		storage:    storage,
		cacheTtl:   cacheTtl,
		httpClient: &http.Client{},
		logger:     logger,
	}

	return &handler, nil
}

func (h *AuthenticationHandler) Authenticate(username string, password string) (string, error) {
	authRequest := h.config.ProviderConfig.Parameters
	authRequest["username"] = username
	authRequest["password"] = password

	jsonString, err := json.Marshal(authRequest)
	if err != nil {
		return "", err
	}

	redactedAuthRequest := authRequest
	redactedAuthRequest["password"] = "*REDACTED*"

	debugJsonString, _ := json.Marshal(redactedAuthRequest)

	h.logger.Info("authenticating user %s", username)
	h.logger.Debug("authentication request: %s", debugJsonString)

	req, err := http.NewRequest("POST", h.config.ProviderConfig.Url+"/authenticate", bytes.NewBuffer(jsonString))
	req.Header.Set("Accept", "application/jwt")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusForbidden {
			h.logger.Warning("invalid credentials for user %s: %s", username, body)
			return "", InvalidCredentialsError
		} else {
			err := fmt.Errorf("unexpected status code %d for user %s: %s", resp.StatusCode, username, body)
			h.logger.Error(err.Error())
			return "", err
		}
	}

	body, _ := ioutil.ReadAll(resp.Body)
	return string(body), nil
}

func (h *AuthenticationHandler) GetVerificationKey() ([]byte, error) {
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
	h.cachedKeyExpiration = time.Now().Add(h.cacheTtl)

	return h.cachedKey, nil
}

func (h *AuthenticationHandler) IsAuthenticated(req *http.Request) (bool, string, error) {
	token, err := h.storage.ReadToken(req)
	if err != nil {
		if err == NoTokenError {
			return false, "", nil
		} else {
			return false, "", err
		}
	}

	key, err := h.GetVerificationKey()
	if err != nil {
		return false, "", err
	}

	var keyFunc jwt.Keyfunc = func(decodedToken *jwt.Token) (interface{}, error) {
		if _, ok := decodedToken.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", decodedToken.Header["alg"])
		}
		return key, nil
	}

	dec, err := jwt.Parse(token, keyFunc)
	if err == nil && dec.Valid {
		return true, token, nil
	}

	acceptableErrors := jwt.ValidationErrorExpired | jwt.ValidationErrorSignatureInvalid
	if err != nil {
		switch t := err.(type) {
		case *jwt.ValidationError:
			if t.Errors&acceptableErrors != 0 {
				return false, "", nil
			}
		}
		return false, "", err
	}

	return false, "", nil
}
