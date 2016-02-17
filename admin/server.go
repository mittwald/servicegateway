package admin

import (
	"encoding/json"
	"fmt"
	"github.com/go-zoo/bone"
	"github.com/mittwald/servicegateway/auth"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"net/url"
)

func writeError(res http.ResponseWriter, msg string) {
	res.WriteHeader(500)
	res.Write([]byte(fmt.Sprintf(`{"msg":"%s"}`, msg)))
}

func NewAdminServer(
	tokenStore auth.TokenStore,
	tokenVerifier *auth.JwtVerifier,
	authHandler *auth.AuthenticationHandler,
	logger *logging.Logger,
) (http.Handler, error) {
	mux := bone.New()

	mux.Get("/tokens", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		tokenStream, err := tokenStore.GetAllTokens()
		if err != nil {
			logger.Error(err.Error())
			writeError(res, "could not load tokens")
			return
		}

		enc := json.NewEncoder(res)
		scheme := "http"

		if req.URL.Scheme != "" {
			scheme = req.URL.Scheme
		}

		res.Write([]byte{'['})
		for v := range tokenStream {
			enc.Encode(TokenJson{
				Jwt:   v.Jwt,
				Token: v.Token,
				Href: fmt.Sprintf("%s://%s/tokens/%s", scheme, req.Host, url.QueryEscape(v.Token)),
			})
		}
		res.Write([]byte{']'})
	}))

	mux.Put("/tokens/#token^(.*)$", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		if req.Header.Get("Content-Type") != "application/jwt" {
			res.WriteHeader(415)
			res.Write([]byte(`{"msg":"only 'application/jwt' is allowed as content-type"}`))
			return
		}

		jwtBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Error("error while loading request body: %s", err)
			writeError(res, "could not read request body")
			return
		}

		jwt := string(jwtBytes)

		valid, _, err := tokenVerifier.VerifyToken(jwt)
		if err != nil || !valid {
			res.WriteHeader(400)
			res.Write([]byte(fmt.Sprintf(`{"msg":"invalid token","reason":"%s"}`, err)))
			return
		}

		tokenString := bone.GetValue(req, "token")

		err = tokenStore.SetToken(tokenString, jwt)
		if err != nil {
			logger.Error("error while storing token: %s", err)
			res.WriteHeader(500)
			res.Write([]byte(`{"msg":"could not store token"}`))
			return
		}

		res.WriteHeader(200)
		res.Write([]byte(fmt.Sprintf(`{"token":"%s"}`, tokenString)))
		return
	}))

	mux.Post("/tokens", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		if req.Header.Get("Content-Type") != "application/jwt" {
			res.WriteHeader(415)
			res.Write([]byte(`{"msg":"only 'application/jwt' is allowed as content-type"}`))
			return
		}

		jwtBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Error("error while loading request body: %s", err)
			writeError(res, "could not read request body")
			return
		}

		jwt := string(jwtBytes)

		valid, _, err := tokenVerifier.VerifyToken(jwt)
		if err != nil || !valid {
			res.WriteHeader(400)
			res.Write([]byte(fmt.Sprintf(`{"msg":"invalid token","reason":"%s"}`, err)))
			return
		}

		tokenString, err := tokenStore.AddToken(jwt)
		if err != nil {
			logger.Error("error while storing token: %s", err)
			res.WriteHeader(500)
			res.Write([]byte(`{"msg":"could not store token"}`))
			return
		}

		res.WriteHeader(200)
		res.Write([]byte(fmt.Sprintf(`{"token":"%s"}`, tokenString)))
		return
	}))

	return mux, nil
}