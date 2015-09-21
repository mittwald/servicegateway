package auth

import "net/http"
import (
	"errors"
	"mittwald.de/servicegateway/config"
	"github.com/garyburd/redigo/redis"
	"fmt"
	"crypto/rand"
	"encoding/base64"
)

var NoTokenError error = errors.New("no authentication token present")

type TokenStorage interface {
	ReadToken(*http.Request) (string, error)
	WriteToken(http.ResponseWriter, string) error
}

type CookieTokenStorage struct {
	Config *config.StorageAuthConfig
}

type HeaderTokenStorage struct {
	Config *config.StorageAuthConfig
}

type SessionTokenStorage struct {
	Config *config.StorageAuthConfig
	RedisPool *redis.Pool
}

func (c *CookieTokenStorage) ReadToken(req *http.Request) (string, error) {
	cookie, err := req.Cookie(c.Config.Name)
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
		Name: c.Config.Name,
		Value: token,
		MaxAge: 0,
		Secure: c.Config.CookieSecure,
		HttpOnly: c.Config.CookieHttpOnly,
		Domain: c.Config.CookieDomain,
		Path: "/",
	}

	http.SetCookie(res, &cookie)
	return nil
}

func (h *HeaderTokenStorage) ReadToken(req *http.Request) (string, error) {
	header, ok := req.Header[h.Config.Name]
	if ok {
		return header[0], nil
	} else {
		return "", NoTokenError
	}
}

func (h *HeaderTokenStorage) WriteToken(res http.ResponseWriter, token string) error {
	res.Header().Set(h.Config.Name, token)
	return nil
}

func (s *SessionTokenStorage) ReadToken(req *http.Request) (string, error) {
	conn := s.RedisPool.Get()
	defer conn.Close()
	sessionCookie, err := req.Cookie(s.Config.Name)

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
	conn := s.RedisPool.Get()
	defer conn.Close()

	if _, err := rand.Read(rb); err != nil {
		return err
	}

	sessionId := base64.URLEncoding.EncodeToString(rb)
	sessionKey := fmt.Sprintf("token_%s", sessionId)

	conn.Do("SET", sessionKey, token)
	cookie := http.Cookie{
		Name: s.Config.Name,
		Value: sessionId,
		MaxAge: 0,
		Secure: s.Config.CookieSecure,
		HttpOnly: s.Config.CookieHttpOnly,
		Domain: s.Config.CookieDomain,
		Path: "/",
	}
	http.SetCookie(res, &cookie)
	return nil
}