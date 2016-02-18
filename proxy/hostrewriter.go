package proxy
import (
	"net/http"
	"strings"
	"encoding/json"
	"net/http/httptest"
	"io/ioutil"
	"bufio"
	"fmt"
	"strconv"
	"github.com/op/go-logging"
	"regexp"
	"net/url"
	"errors"
	"github.com/julienschmidt/httprouter"
)

var UnmappableUrl = errors.New("unmappable URL")

type HostRewriter interface {
	CanHandle(http.ResponseWriter) bool
	Rewrite([]byte, *url.URL) ([]byte, error)
	Decorate(httprouter.Handle) httprouter.Handle
}

type mapping struct {
	regex *regexp.Regexp
	repl func([]string) string
}

type JsonHostRewriter struct {
	InternalHost string
	PublicHost string
	SearchKeys []string
	Mappings []mapping
	Logger *logging.Logger
}

func NewHostRewriter(internalHost, publicHost string, searchKeys []string, urlPatterns map[string]string, logger *logging.Logger) (HostRewriter, error) {
	mappings := make([]mapping, len(urlPatterns))
	i := 0

	for sourcePattern, targetPattern := range urlPatterns {
		re := regexp.MustCompile(sourcePattern)
		replacements := make(map[int]*regexp.Regexp, len(re.SubexpNames()))

		for i, name := range re.SubexpNames() {
			if name != "" {
				fmt.Printf("replacement %d: %s\n", i, ":" + name)
				replacements[i] = regexp.MustCompile(":" + name)
			}
		}

		mappings[i] = mapping {
			regex: re,
			repl: func(matches []string) string {
				path := targetPattern
				fmt.Println(matches)
				for k, v := range replacements {
					fmt.Printf("path: %s, pattern %s\n", path, v.String())
					path = v.ReplaceAllString(path, matches[k])
				}
				return path
			},
		}

		i += 1
	}

	return &JsonHostRewriter{
		InternalHost: internalHost,
		PublicHost: publicHost,
		Mappings: mappings,
		Logger: logger,
	}, nil
}

func (j *JsonHostRewriter) Decorate(handler httprouter.Handle) httprouter.Handle {
	return func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		req.Header.Del("Accept-Encoding")

		recorder := httptest.NewRecorder()
		handler(recorder, req, params)

		if j.CanHandle(recorder) {
			b, err := ioutil.ReadAll(recorder.Body)
			if err != nil {
				j.Logger.Error("error while reading response body: %s", err)
				rw.WriteHeader(500)
				rw.Write([]byte(`{"msg":"internal server error"}`))
			}

			fmt.Println(b)
			fmt.Println(string(b))

			url := *req.URL
			url.Host = req.Host

			if url.Scheme == "" {
				url.Scheme = "http"
			}

			b, err = j.Rewrite(b, &url)
			if err != nil {
				j.Logger.Error("error while rewriting response body: %s", err)
				rw.WriteHeader(500)
				rw.Write([]byte(`{"msg":"internal server error"}`))
				return
			}

			fmt.Printf("Rewrote response body, now %d bytes\n", len(b))

			for k, _ := range recorder.Header() {
				rw.Header()[k] = recorder.Header()[k]
			}

			rw.Header().Set("Content-Length", strconv.Itoa(len(b)))
			rw.WriteHeader(recorder.Code)
			rw.Write(b)
		} else {
			for k, _ := range recorder.Header() {
				rw.Header()[k] = recorder.Header()[k]
			}
			rw.WriteHeader(recorder.Code)
			reader := bufio.NewReader(recorder.Body)
			_, err := reader.WriteTo(rw)
			if err != nil {
				fmt.Printf("GUBBEL %s", err)
			}
		}
	}
}

func (j *JsonHostRewriter) CanHandle(res http.ResponseWriter) bool {
	return strings.HasPrefix(res.Header().Get("content-type"), "application/json")
}

func (j *JsonHostRewriter) Rewrite(body []byte, reqUrl *url.URL) ([]byte, error) {
	//jsonData := new(interface{})
	var jsonData interface{}
	err := json.Unmarshal(body, &jsonData)

	if err != nil {
		return nil, err
	}

	jsonData, err = j.walkJson(jsonData, reqUrl)

	reencoded, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	return reencoded, nil
}

func (j *JsonHostRewriter) rewriteUrl(urlString string, reqUrl *url.URL) (string, error) {
	parsedUrl, err := url.Parse(urlString)
	if err != nil {
		return urlString, fmt.Errorf("error while parsing url %s: %s", urlString, err)
	}

	for _, mapping := range j.Mappings {
		matches := mapping.regex.FindStringSubmatch(parsedUrl.Path)
		if matches != nil {
			parsedUrl.Host = reqUrl.Host
			parsedUrl.Path = mapping.repl(matches)
			return parsedUrl.String(), nil
		}
	}

	return "", UnmappableUrl
}

func (j *JsonHostRewriter) walkJson(jsonStruct interface{}, reqUrl *url.URL) (interface{}, error) {
	switch typed := jsonStruct.(type) {
	case map[string]interface{}:
		for key, _ := range typed {
			if key == "href" {
				if url, ok := typed["href"].(string); ok {
					newUrl, err := j.rewriteUrl(url, reqUrl)
					if err == UnmappableUrl {
						delete(typed, "href")
					} else if err != nil {
						j.Logger.Error("error while mapping url %s: %s", url, err)
						delete(typed, "href")
					} else {
						typed["href"] = newUrl
					}
				}
			} else {
				v, err := j.walkJson(typed[key], reqUrl)
				if err != nil {
					return nil, err
				}

				typed[key] = v
			}
		}
		return typed, nil

	case []interface{}:
		for key, _ := range typed {
			v, err := j.walkJson(typed[key], reqUrl)
			if err != nil {
				return nil, err
			}

			typed[key] = v
		}
		return typed, nil
	}

	return jsonStruct, nil
}

