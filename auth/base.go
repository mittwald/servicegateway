package auth

import (
	"net/http"
	"mittwald.de/servicegateway/config"
	"errors"
	"fmt"
	"io/ioutil"
	"time"
	"sync"
	"encoding/json"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/go-zoo/bone"
	"bytes"
	logging "github.com/op/go-logging"
	"github.com/garyburd/redigo/redis"
)

var InvalidCredentialsError error = errors.New("invalid credentials given")

type AuthDecorator interface {
	DecorateHandler(http.HandlerFunc) http.HandlerFunc
	RegisterRoutes(*bone.Mux) error
}

type AuthenticationHandler struct {
	config *config.GlobalAuth
	storage TokenStorage
	cacheTtl            time.Duration
	cachedKey           []byte
	cachedKeyExpiration time.Time
	cachedKeyLock       sync.Mutex
	httpClient *http.Client
	logger *logging.Logger
}

type AuthenticationRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TimeToLive int `json:"ttl"`
	Providers []string `json:"providers"`
}

func NewAuthenticationHandler(cfg *config.GlobalAuth, redisPool *redis.Pool, logger *logging.Logger) (*AuthenticationHandler, error) {
	var storage TokenStorage

	switch cfg.StorageConfig.Mode {
	case "cookie":
		storage = &CookieTokenStorage{Config: &cfg.StorageConfig}
	case "header":
		storage = &HeaderTokenStorage{Config: &cfg.StorageConfig}
	case "session":
		storage = &SessionTokenStorage{Config: &cfg.StorageConfig, RedisPool: redisPool}
	default:
		return nil, errors.New(fmt.Sprintf("unsupported token storage mode: '%s'", cfg.StorageConfig.Mode))
	}

	cacheTtl, err := time.ParseDuration(cfg.KeyCacheTtl)
	if err != nil {
		return nil, err
	}

	handler := AuthenticationHandler{
		config: cfg,
		storage: storage,
		cacheTtl: cacheTtl,
		httpClient: &http.Client{},
		logger: logger,
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

	h.logger.Debug("request body: %s", jsonString)

	req, err := http.NewRequest("POST", h.config.ProviderConfig.Url + "/authenticate", bytes.NewBuffer(jsonString))
	req.Header.Set("Accept", "application/jwt")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		body, _ := ioutil.ReadAll(resp.Body)
		h.logger.Error("error while trying to authenticate %s: %s", username, body)
		return "", InvalidCredentialsError
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
	} else {
		h.cachedKeyLock.Lock()
		defer h.cachedKeyLock.Unlock()

		if h.cachedKey != nil && h.cachedKeyExpiration.After(time.Now()) {
			return h.cachedKey, nil
		}

		resp, err := http.Get(h.config.VerificationKeyUrl)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err))
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Could not retrieve key from '%s': %s", h.config.VerificationKeyUrl, err))
		}

		h.cachedKey = body
		h.cachedKeyExpiration = time.Now().Add(h.cacheTtl)

		return h.cachedKey, nil
	}
}

func (h *AuthenticationHandler) IsAuthenticated(req *http.Request) (bool, error) {
	token, err := h.storage.ReadToken(req)
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

	var keyFunc jwt.Keyfunc = func(decodedToken *jwt.Token) (interface{}, error) {
		fmt.Println(decodedToken)
		if _, ok := decodedToken.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %s", decodedToken.Header["alg"])
		}
		return key, nil
	}

	dec, err := jwt.Parse(token, keyFunc)
	if err == nil && dec.Valid {
		return true, nil
	}

	acceptableErrors := jwt.ValidationErrorExpired | jwt.ValidationErrorSignatureInvalid
	if err != nil {
		switch t := err.(type) {
		case *jwt.ValidationError:
			if t.Errors & acceptableErrors != 0 {
				return false, nil
			}
		}
		return false, err
	}

	return false, nil
}

func NewAuthDecorator(authConfig *config.GlobalAuth, redisPool *redis.Pool, logger *logging.Logger) (AuthDecorator, error) {
	authHandler, err := NewAuthenticationHandler(authConfig, redisPool, logger)
	if err != nil {
		return nil, err
	}

	switch authConfig.Mode {
	case "graphical":
		return &GraphicalAuthDecorator{
			authHandler,
			authConfig,
			logger,
		}, nil
	case "rest":
		return &RestAuthDecorator{
			authHandler,
		}, nil
	}
	return nil, errors.New(fmt.Sprintf("unsupported authentication mode: '%s'", authConfig.Mode))
}
