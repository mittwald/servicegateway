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
	"time"
)

func writeError(res http.ResponseWriter, msg string) {
	res.WriteHeader(500)
	_, _ = res.Write([]byte(fmt.Sprintf(`{"msg":"%s"}`, msg)))
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
			logger.Error(err)
			writeError(res, "could not load tokens")
			return
		}

		enc := json.NewEncoder(res)
		scheme := "http"

		if req.URL.Scheme != "" {
			scheme = req.URL.Scheme
		}

		_, _ = res.Write([]byte{'['})
		for v := range tokenStream {
			err := enc.Encode(TokenJson{
				Jwt:   v.Jwt,
				Token: v.Token,
				Href:  fmt.Sprintf("%s://%s/tokens/%s", scheme, req.Host, url.QueryEscape(v.Token)),
			})
			if err != nil {
				logger.Error(err)
				writeError(res, "could not encode tokens")
				return
			}
		}
		_, _ = res.Write([]byte{']'})
	}))

	mux.Put("/tokens/#token^(.*)$", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		if req.Header.Get("Content-Type") != "application/jwt" {
			res.WriteHeader(415)
			_ , _ = res.Write([]byte(`{"msg":"only 'application/jwt' is allowed as content-type"}`))
			return
		}

		jwtBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Errorf("error while loading request body: %s", err)
			writeError(res, "could not read request body")
			return
		}

		jwt := string(jwtBytes)

		valid, _, _, err := tokenVerifier.VerifyToken(jwt)
		if err != nil || !valid {
			res.WriteHeader(400)
			_ , _ = res.Write([]byte(fmt.Sprintf(`{"msg":"invalid token","reason":"%s"}`, err)))
			return
		}

		tokenString := bone.GetValue(req, "token")

		exp, err := tokenStore.SetToken(tokenString, &auth.JWTResponse{JWT: jwt})
		if err != nil {
			logger.Errorf("error while storing token: %s", err)
			res.WriteHeader(500)
			_ , _ = res.Write([]byte(`{"msg":"could not store token"}`))
			return
		}

		res.WriteHeader(200)

		if exp != 0 {
			_ , _ = res.Write([]byte(fmt.Sprintf(`{"token":"%s","expires":"%s"}`, tokenString, time.Unix(exp, 0).Format(time.RFC3339))))
		} else {
			_ , _ = res.Write([]byte(fmt.Sprintf(`{"token":"%s"}`, tokenString)))
		}
	}))

	mux.Post("/tokens", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")

		if req.Header.Get("Content-Type") != "application/jwt" {
			res.WriteHeader(415)
			_ , _ = res.Write([]byte(`{"msg":"only 'application/jwt' is allowed as content-type"}`))
			return
		}

		jwtBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			logger.Errorf("error while loading request body: %s", err)
			writeError(res, "could not read request body")
			return
		}

		jwt := string(jwtBytes)

		valid, _, _, err := tokenVerifier.VerifyToken(jwt)
		if err != nil || !valid {
			res.WriteHeader(400)
			_ , _ = res.Write([]byte(fmt.Sprintf(`{"msg":"invalid token","reason":"%s"}`, err)))
			return
		}

		tokenString, exp, err := tokenStore.AddToken(&auth.JWTResponse{JWT: jwt})
		if err != nil {
			logger.Errorf("error while storing token: %s", err)
			res.WriteHeader(500)
			_ , _ = res.Write([]byte(`{"msg":"could not store token"}`))
			return
		}

		res.WriteHeader(200)
		if exp != 0 {
			_ , _ = res.Write([]byte(fmt.Sprintf(`{"token":"%s","expires":"%s"}`, tokenString, time.Unix(exp, 0).Format(time.RFC3339))))
		} else {
			_ , _ = res.Write([]byte(fmt.Sprintf(`{"token":"%s"}`, tokenString)))
		}
	}))

	return mux, nil
}
