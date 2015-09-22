package proxy

import (
	"github.com/bluele/gcache"
	"net/http"
	"fmt"
	"bytes"
	"io/ioutil"
)

type Cache struct {
	cache gcache.Cache
}

type ResponseBuffer struct {
	body []byte
	buf *bytes.Buffer
	header http.Header
	status int
	complete bool
}

func NewResponseBuffer() *ResponseBuffer {
	b := new(ResponseBuffer)
	b.buf = bytes.NewBuffer(make([]byte, 0, 4096))
	b.status = 200
	b.complete = false
	b.header = http.Header{}
	return b
}


func (r *ResponseBuffer) Header() http.Header {
	return r.header
}

func (r *ResponseBuffer) WriteHeader(status int) {
	r.status = status
}

func (r *ResponseBuffer) Write(b []byte) (int, error) {
	l, err := r.buf.Write(b)
	return l, err
}

func (r *ResponseBuffer) Complete() {
	r.body, _ = ioutil.ReadAll(r.buf)
	r.complete = true
}

func (r *ResponseBuffer) Dump(rw http.ResponseWriter) {
	for key, values := range r.header {
		for _, value := range values {
			rw.Header().Add(key, value)
		}
	}

	rw.Write(r.body)
}

func NewCache(s int) *Cache {
	c := new(Cache)
	c.cache = gcache.New(s).LRU().Build()
	return c
}

func (c *Cache) identifierForRequest(req *http.Request) string {
	identifier := req.RequestURI

	if accept := req.Header.Get("Accept"); accept != "" {
		identifier += accept
	}

	return identifier
}

func (c *Cache) DecorateUnsafeHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		identifier := c.identifierForRequest(req)
		c.cache.Remove(identifier)
		rw.Header().Add("X-Cache", "PURGED")
		handler(rw, req)
	}
}

func (c *Cache) DecorateHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		identifier := c.identifierForRequest(req)

		useCache := true
		if req.Header.Get("Cache-Control") == "no-cache" {
			useCache = false
		}

		entry, err := c.cache.Get(identifier)
		if useCache == false || err == gcache.NotFoundKeyError {
			buf := NewResponseBuffer()

			handler(buf, req)
			buf.Complete()

			if useCache {
				rw.Header().Add("X-Cache", "MISS")
				c.cache.Set(identifier, buf)
			} else {
				rw.Header().Add("X-Cache", "PASS")
			}

			buf.Dump(rw)
		} else if err == nil {
			switch entry := entry.(type) {
			case *ResponseBuffer:
				rw.Header().Add("X-Cache", "HIT")
				entry.Dump(rw)
			default:
				fmt.Println("Unknown type in cache")
				rw.WriteHeader(500)
				rw.Write([]byte("{\"msg\":\"internal server error\"}"))
			}
		} else {
			rw.WriteHeader(500)
			rw.Write([]byte("{\"msg\":\"internal server error\"}"))
		}

	}
}
