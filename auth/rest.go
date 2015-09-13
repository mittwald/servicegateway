package auth

import "net/http"

type RestAuthDecorator struct {
	authHandler *AuthenticationHandler
}

func (a *RestAuthDecorator) DecorateHandler(orig http.HandlerFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		authenticated, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			res.WriteHeader(503)
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
		} else if ! authenticated {
			res.WriteHeader(403)
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte("{\"msg\": \"not authenticated\"}"))
		} else {
			orig(res, req)
		}
	}
}

