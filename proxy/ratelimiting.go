package proxy

import (
	"net/http"
	"sync"
	"strconv"
	"fmt"
	"net"
	"time"
	"mittwald.de/charon/config"
)

type Bucket struct {
	sync.Mutex
	limit int
	fillLevel int
}

type RateThrottler struct {
	buckets map[string]*Bucket
	burstSize int
	requestsPerSecond int
}

func NewThrottler(cfg config.RateLimiting) *RateThrottler {
	t := new(RateThrottler)
	t.buckets = make(map[string]*Bucket)
	t.burstSize = cfg.Burst
	t.requestsPerSecond = cfg.RequestsPerSecond

	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for _ = range ticker.C {
			t.FillBuckets()
		}
	}()

	return t
}

func (t *RateThrottler) identifyClient(req *http.Request) string {
	addr, _ := net.ResolveTCPAddr("tcp", req.RemoteAddr)
	return addr.IP.String()
}

func (t *RateThrottler) FillBuckets() {
	for user, bucket := range t.buckets {
		bucket.Lock()
		defer bucket.Unlock()

		if bucket.fillLevel < bucket.limit {
			bucket.fillLevel ++
		}

		if bucket.fillLevel == bucket.limit {
			fmt.Println("Purging bucket " + user)
			delete(t.buckets, user)
		}
	}
}

func (t *RateThrottler) TakeToken(user string) Bucket {
	bucket, ok := t.buckets[user]
	if ! ok {
		bucket = new(Bucket)
		bucket.fillLevel = t.burstSize
		bucket.limit = t.burstSize
		t.buckets[user] = bucket
	}

	bucket.Lock()
	defer bucket.Unlock()

	if bucket.fillLevel > 0 {
		bucket.fillLevel -= 1
	}

	return copy(*bucket)
}

func (t *RateThrottler) DecorateHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		user := t.identifyClient(req)
		bucket := t.TakeToken(user)

		rw.Header().Add("X-RateLimit", strconv.Itoa(bucket.limit))
		rw.Header().Add("X-RateLimit-Remaining", strconv.Itoa(bucket.fillLevel))

		if bucket.fillLevel <= 0 {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(429)
			rw.Write([]byte("{\"msg\":\"rate limit exceeded\"}"))
		} else {
			handler(rw, req)
		}
	}
}
