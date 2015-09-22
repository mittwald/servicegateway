package auth

import (
	"net/http"
	"mittwald.de/servicegateway/config"
	"github.com/go-zoo/bone"
	"html/template"
	"fmt"
	"net/url"
	"github.com/op/go-logging"
)

type GraphicalAuthDecorator struct {
	authHandler *AuthenticationHandler
	config *config.GlobalAuth
	logger *logging.Logger
}

type LoginResult struct {
	Redirect string
	Errors struct {
		InvalidCredentials bool
		UserEmpty bool
		PasswordEmpty bool
    }
}

func (r *LoginResult) HasErrors() bool {
	return r.Errors.UserEmpty ||
		r.Errors.PasswordEmpty ||
		r.Errors.InvalidCredentials
}

func (a *GraphicalAuthDecorator) DecorateHandler(orig http.HandlerFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		authenticated, err := a.authHandler.IsAuthenticated(req)
		if err != nil {
			a.logger.Error(err.Error())
			res.Header().Set("Content-Type", "application/json")
			res.Write([]byte("{\"msg\": \"service unavailable\"}"))
			res.WriteHeader(503)
		} else if ! authenticated {
			target := fmt.Sprintf("%s?redirect=%s", a.config.GraphicalConfig.LoginRoute, url.QueryEscape(req.URL.String()))

			res.Header().Set("Content-Type", "application/json")
			res.Header().Set("Location", target)
			res.WriteHeader(303)
			res.Write([]byte("{\"msg\": \"authentication required\"}"))
		} else {
			orig(res, req)
		}
	}
}

func (a *GraphicalAuthDecorator) RegisterRoutes(mux *bone.Mux) error {
	tmpl, err := template.ParseFiles("templates/login.html", "templates/layout.html")
	if err != nil {
		return err
	}

	fileserver := http.FileServer(http.Dir("./static/"))
	mux.Get("/_static/", http.StripPrefix("/_static/", fileserver))

	mux.GetFunc(a.config.GraphicalConfig.LoginRoute, func(res http.ResponseWriter, req *http.Request) {
		result := LoginResult{}

		if len(req.URL.Query()["redirect"]) > 0 {
			result.Redirect = req.URL.Query()["redirect"][0]
		}

		err := tmpl.ExecuteTemplate(res, "layout", &result)
		if err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			res.Write([]byte(err.Error()))
		}
	})

	mux.PostFunc("/_gateway/authenticate", func(res http.ResponseWriter, req *http.Request) {
		username := req.PostFormValue("username")
		password := req.PostFormValue("password")
		redirect := req.PostFormValue("redirect")

		result := LoginResult{Redirect: redirect}

		if username == "" {
			result.Errors.UserEmpty = true
		}

		if password == "" {
			result.Errors.PasswordEmpty = true
		}

		if result.HasErrors() {
			fmt.Println(result)
			tmpl.ExecuteTemplate(res, "layout", &result)
			return
		}

		token, err := a.authHandler.Authenticate(username, password)
		if err != nil {
			result.Errors.InvalidCredentials = true
			res.WriteHeader(http.StatusUnauthorized)
			tmpl.ExecuteTemplate(res, "layout", &result)
			return
		}

		a.authHandler.storage.WriteToken(res, token)
		if redirect != "" {
			res.Header().Set("Location", redirect)
			res.WriteHeader(303)
			res.Write([]byte("Successfully authenticated. Redirecting to original request."))
		} else {
			res.Write([]byte("Hello."))
		}
	})

	return nil
}