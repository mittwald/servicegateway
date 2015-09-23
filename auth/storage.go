package auth

import "net/http"
import (
	"errors"
	"mittwald.de/servicegateway/config"
	"github.com/garyburd/redigo/redis"
	"fmt"
	"crypto/rand"
	"encoding/base64"
	"github.com/op/go-logging"
)

var NoTokenError error = errors.New("no authentication token present")

type TokenStorage interface {
	ReadToken(*http.Request) (string, error)
	WriteToken(http.ResponseWriter, string) error
	WriteTokenToUpstreamRequest(*http.Request, string) error
}

type NoOpTokenStorage struct {}

type CookieTokenStorage struct {
	cfg *config.StorageAuthConfig
	log *logging.Logger
}

type HeaderTokenStorage struct {
	cfg *config.StorageAuthConfig
	log *logging.Logger
}

type SessionTokenStorage struct {
	cfg *config.StorageAuthConfig
	log *logging.Logger
	redisPool *redis.Pool
}

func (n *NoOpTokenStorage) ReadToken(req *http.Request) (string, error) {
	return "", NoTokenError
}

func (n *NoOpTokenStorage) WriteToken(res http.ResponseWriter, token string) error {
	return nil
}

func (n *NoOpTokenStorage) WriteTokenToUpstreamRequest(res *http.Request, token string) error {
	return nil
}

func (c *CookieTokenStorage) ReadToken(req *http.Request) (string, error) {
	cookie, err := req.Cookie(c.cfg.Name)
	if err != nil {
		if err == http.ErrNoCookie {
			return "", NoTokenError
		} else {
			return "", err
		}
	}

	return cookie.Value, nil
}

func (c *CookieTokenStorage) WriteToken(res http.ResponseWriter, token string) error {
	cookie := http.Cookie{
		Name: c.cfg.Name,
		Value: token,
		MaxAge: 0,
		Secure: c.cfg.CookieSecure,
		HttpOnly: c.cfg.CookieHttpOnly,
		Domain: c.cfg.CookieDomain,
		Path: "/",
	}

	http.SetCookie(res, &cookie)
	return nil
}

func (c *CookieTokenStorage) WriteTokenToUpstreamRequest(req *http.Request, token string) error {
	if _, err := req.Cookie(c.cfg.Name); err == http.ErrNoCookie {
		val, ok := req.Header[c.cfg.Name]; if ok {
			req.Header.Set("Cookie", fmt.Sprintf("%s; %s=%s", val, c.cfg.Name, token))
		} else {
			req.Header.Set("Cookie", fmt.Sprintf("%s=%s", c.cfg.Name, token))
		}
	}
	return nil
}

func (h *HeaderTokenStorage) ReadToken(req *http.Request) (string, error) {
	header, ok := req.Header[h.cfg.Name]
	if ok {
		return header[0], nil
	} else {
		return "", NoTokenError
	}
}

func (h *HeaderTokenStorage) WriteToken(res http.ResponseWriter, token string) error {
	res.Header().Set(h.cfg.Name, token)
	return nil
}

func (h *HeaderTokenStorage) WriteTokenToUpstreamRequest(req *http.Request, token string) error {
	req.Header.Set(h.cfg.Name, token)
	return nil
}

func (s *SessionTokenStorage) ReadToken(req *http.Request) (string, error) {
	conn := s.redisPool.Get()
	defer conn.Close()
	sessionCookie, err := req.Cookie(s.cfg.Name)

	if err != nil {
		if err == http.ErrNoCookie {
			return "", NoTokenError
		} else {
			return "", err
		}
	}

	sessionId := sessionCookie.Value
	sessionKey := fmt.Sprintf("token_%s", sessionId)
	jwt, err := redis.String(conn.Do("GET", sessionKey))
	if err != nil {
		return "", err
	}

	return jwt, nil
}

func (s *SessionTokenStorage) WriteToken(res http.ResponseWriter, token string) error {
	rb := make([]byte, 32)
	conn := s.redisPool.Get()
	defer conn.Close()

	if _, err := rand.Read(rb); err != nil {
		return err
	}

	sessionId := base64.URLEncoding.EncodeToString(rb)
	sessionKey := fmt.Sprintf("token_%s", sessionId)

	s.log.Debug("Starting session %s", sessionId)

	conn.Do("SET", sessionKey, token)
	cookie := http.Cookie{
		Name: s.cfg.Name,
		Value: sessionId,
		MaxAge: 0,
		Secure: s.cfg.CookieSecure,
		HttpOnly: s.cfg.CookieHttpOnly,
		Domain: s.cfg.CookieDomain,
		Path: "/",
	}
	http.SetCookie(res, &cookie)
	return nil
}

func (s *SessionTokenStorage) WriteTokenToUpstreamRequest(req *http.Request, token string) error {
	return errors.New("session token storage is unsupported for upstream services")
}