package auth

import (
	"github.com/go-zoo/bone"
	"mittwald.de/servicegateway/config"
	"net/http"
)

type RestAuthDecorator struct {
	authHandler *AuthenticationHandler
}

func (a *RestAuthDecorator) DecorateHandler(orig http.Handler, appCfg *config.Application) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		authenticated, _, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(503)
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
		} else if !authenticated {
			res.Header().Set("Content-Type", "application/json")
			res.WriteHeader(403)
			res.Write([]byte("{\"msg\": \"not authenticated\"}"))
		} else {
			orig.ServeHTTP(res, req)
		}
	})
}

func (a *RestAuthDecorator) RegisterRoutes(mux *bone.Mux) error {
	return nil
}
