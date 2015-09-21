package auth

import "net/http"
import (
	"errors"
	"mittwald.de/servicegateway/config"
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