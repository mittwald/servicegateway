package proxy

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"github.com/op/go-logging"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var UnmappableURL = errors.New("unmappable URL")
var RemoveElement = errors.New("Obsolete element")

type HostRewriter interface {
	CanHandle(http.ResponseWriter) bool
	Rewrite([]byte, *url.URL) ([]byte, error)
	RewriteURL(urlString string, reqURL *url.URL) (string, error)
	Decorate(httprouter.Handle) httprouter.Handle
}

type mapping struct {
	regex         *regexp.Regexp
	targetPattern string
	replacements  map[int]*regexp.Regexp
}

type JSONHostRewriter struct {
	InternalHost string
	Mappings     []mapping
	Logger       *logging.Logger
}

func (m *mapping) repl(matches []string) string {
	path := m.targetPattern
	for k, _ := range m.replacements {
		path = m.replacements[k].ReplaceAllString(path, matches[k])
	}
	return path
}

func NewHostRewriter(internalHost string, urlPatterns map[string]string, logger *logging.Logger) (HostRewriter, error) {
	mappings := make([]mapping, len(urlPatterns))
	i := 0

	for sourcePattern, targetPattern := range urlPatterns {
		re := regexp.MustCompile(sourcePattern)
		replacements := make(map[int]*regexp.Regexp, len(re.SubexpNames()))

		for i, name := range re.SubexpNames() {
			if name != "" {
				replacements[i] = regexp.MustCompile(":" + name)
			}
		}

		mappings[i] = mapping{
			regex:         re,
			targetPattern: targetPattern,
			replacements:  replacements,
		}

		i += 1
	}

	return &JSONHostRewriter{
		InternalHost: internalHost,
		Mappings:     mappings,
		Logger:       logger,
	}, nil
}

func (j *JSONHostRewriter) Decorate(handler httprouter.Handle) httprouter.Handle {
	return func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		if req.Header.Get("X-No-Rewrite") != "" {
			j.Logger.Noticef("skipping json rewriting due to client request")
			handler(rw, req, params)
			return
		}

		publicURL := *req.URL
		publicURL.Host = req.Host

		if req.Header.Get("X-Forwarded-Proto") == "https" || req.Header.Get("X-Forwarded-Proto") == "http" {
			publicURL.Scheme = req.Header.Get("X-Forwarded-Proto")
		} else if publicURL.Scheme == "" {
			publicURL.Scheme = "https"
		}

		req.Header.Del("Accept-Encoding")

		recorder := httptest.NewRecorder()
		handler(recorder, req, params)

		if j.CanHandle(recorder) {
			b, err := ioutil.ReadAll(recorder.Body)
			if err != nil {
				j.Logger.Errorf("error while reading response body: %s", err)
				rw.WriteHeader(500)
				rw.Write([]byte(`{"msg":"internal server error"}`))
			}

			if req.Method != "HEAD" {
				b, err = j.Rewrite(b, &publicURL)
				if err != nil {
					j.Logger.Errorf("error while rewriting response body: %s", err)
					rw.WriteHeader(500)
					rw.Write([]byte(`{"msg":"internal server error"}`))
					return
				}
			}

			j.copyAndRewriteHeaders(recorder, rw, &publicURL)

			rw.Header().Set("Content-Length", strconv.Itoa(len(b)))
			rw.WriteHeader(recorder.Code)
			rw.Write(b)
		} else {
			j.copyAndRewriteHeaders(recorder, rw, &publicURL)
			rw.WriteHeader(recorder.Code)
			reader := bufio.NewReader(recorder.Body)
			_, err := reader.WriteTo(rw)

			if err != nil {
				j.Logger.Errorf("error while writing response body after rewriting: %s", err)
			}
		}
	}
}

func (j *JSONHostRewriter) copyAndRewriteHeaders(source http.ResponseWriter, target http.ResponseWriter, publicURL *url.URL) {
	for k, values := range source.Header() {
		if k == "Location" {
			j.Logger.Debugf("found location header")
			for i, _ := range values {
				newURL, err := j.RewriteURL(values[i], publicURL)
				if err != nil {
					j.Logger.Errorf("error while mapping URL from location header %s: %s", values[i], err)
				} else {
					j.Logger.Debugf("found location header")
					values[i] = newURL
				}
			}
		}
		target.Header()[k] = values
	}
}

func (j *JSONHostRewriter) CanHandle(res http.ResponseWriter) bool {
	return strings.HasPrefix(res.Header().Get("content-type"), "application/json")
}

func (j *JSONHostRewriter) Rewrite(body []byte, reqURL *url.URL) ([]byte, error) {
	var jsonData interface{}
	err := json.Unmarshal(body, &jsonData)

	if err != nil {
		return nil, err
	}

	jsonData, err = j.walkJson(jsonData, reqURL, false)

	reencoded, err := json.Marshal(jsonData)
	if err != nil {
		return nil, err
	}

	return reencoded, nil
}

func (j *JSONHostRewriter) RewriteURL(urlString string, reqURL *url.URL) (string, error) {
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		return urlString, fmt.Errorf("error while parsing url %s: %s", urlString, err)
	}

	for _, mapping := range j.Mappings {
		matches := mapping.regex.FindStringSubmatch(parsedURL.Path)
		if matches != nil {
			parsedURL.Host = reqURL.Host
			parsedURL.Scheme = reqURL.Scheme
			parsedURL.Path = mapping.repl(matches)
			return parsedURL.String(), nil
		}
	}

	return "", UnmappableURL
}

func (j *JSONHostRewriter) walkJson(jsonStruct interface{}, reqURL *url.URL, inLinks bool) (interface{}, error) {
	switch typed := jsonStruct.(type) {
	case map[string]interface{}:
		for key, _ := range typed {
			if key == "href" {
				if url, ok := typed["href"].(string); ok {
					newUrl, err := j.RewriteURL(url, reqURL)
					if err == UnmappableURL {
						delete(typed, "href")
						if inLinks {
							return nil, RemoveElement
						}
					} else if err != nil {
						j.Logger.Errorf("error while mapping url %s: %s", url, err)
						delete(typed, "href")
					} else {
						typed["href"] = newUrl
					}
				}
			} else {
				l := inLinks || (key == "links" || key == "_links")

				v, err := j.walkJson(typed[key], reqURL, l)
				if err == RemoveElement {
					delete(typed, key)
				} else if err != nil {
					return nil, err
				} else {
					typed[key] = v
				}
			}
		}
		return typed, nil

	case []interface{}:
		outputList := make([]interface{}, 0, len(typed))
		removedCount := 0

		for key, _ := range typed {
			v, err := j.walkJson(typed[key], reqURL, inLinks)
			if err == RemoveElement {
				removedCount += 1
			} else if err != nil {
				return nil, err
			} else {
				outputList = append(outputList, v)
			}
		}

		if len(outputList) == 0 {
			return nil, RemoveElement
		}
		return outputList, nil
	}

	return jsonStruct, nil
}
