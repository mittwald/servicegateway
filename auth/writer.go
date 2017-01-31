package auth

import "net/http"

type TokenWriter interface {
	WriteTokenToRequest(string, *http.Request) error
}

type HeaderTokenWriter struct {
	HeaderName string
}

type AuthorizationTokenWriter struct{}

func (h *HeaderTokenWriter) WriteTokenToRequest(jwt string, req *http.Request) error {
	req.Header.Set(h.HeaderName, jwt)
	return nil
}

func (a *AuthorizationTokenWriter) WriteTokenToRequest(jwt string, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+jwt)
	return nil
}
