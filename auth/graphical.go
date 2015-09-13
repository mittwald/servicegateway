package auth

import (
	"net/http"
	"mittwald.de/charon/config"
)

type GraphicalAuthDecorator struct {
	authHandler *AuthenticationHandler
	config *config.GlobalAuth
}

func (a *GraphicalAuthDecorator) DecorateHandler(orig http.HandlerFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		authenticated, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			res.WriteHeader(503)
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
		} else if ! authenticated {
			res.WriteHeader(303)
			res.Header().Set("Content-Type", "application/json")
			res.Header().Set("Location", a.config.GraphicalConfig.LoginRoute)
			res.Write([]byte("{\"msg\": \"authentication required\"}"))
		} else {
			orig(res, req)
		}
	}
}

