package ratelimit

import (
	"github.com/garyburd/redigo/redis"
	logging "github.com/op/go-logging"
	"github.com/mittwald/servicegateway/config"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Bucket struct {
	sync.Mutex
	limit     int
	fillLevel int
}

type RateLimitingMiddleware interface {
	DecorateHandler(handler http.Handler) http.Handler
}

type RedisSimpleRateThrottler struct {
	burstSize         int64
	window            time.Duration
	requestsPerSecond int64
	redisPool         *redis.Pool
	logger            *logging.Logger
}

func NewRateLimiter(cfg config.RateLimiting, red *redis.Pool, logger *logging.Logger) (RateLimitingMiddleware, error) {
	t := new(RedisSimpleRateThrottler)
	t.burstSize = int64(cfg.Burst)
	t.requestsPerSecond = int64(cfg.RequestsPerSecond)
	t.redisPool = red
	t.logger = logger

	if w, err := time.ParseDuration(cfg.Window); err != nil {
		return nil, err
	} else {
		t.window = w
	}

	logger.Info("Initialize rate limiter (burst size %d)", t.burstSize)

	return t, nil
}

func (t *RedisSimpleRateThrottler) identifyClient(req *http.Request) string {
	addr, _ := net.ResolveTCPAddr("tcp", req.RemoteAddr)
	return addr.IP.String()
}

func (t *RedisSimpleRateThrottler) takeToken(user string) (int, int, error) {
	key := "RL_BUCKET_" + user
	conn := t.redisPool.Get()

	conn.Send("MULTI")
	conn.Send("SET", key, t.burstSize, "EX", t.window.Seconds(), "NX")
	conn.Send("DECR", key)

	if val, err := redis.Values(conn.Do("EXEC")); err != nil {
		return 0, 0, err
	} else {
		return int(val[1].(int64)), int(t.burstSize), nil
	}
}

//func (t *RateThrottler) TakeToken(user string) (int, int, error) {
//	key := "RL_BUCKET_" + user
//	keyLA := "RL_BUCKET_LASTACCESS_" + user
//	conn := t.redisPool.Get()
//	now := time.Now().Unix()
//
//	conn.Send("MULTI")
//	conn.Send("SETNX", key, t.burstSize)
//	conn.Send("GETSET", keyLA, now)
//	conn.Send("GET", key)
//
//	if val, err := redis.Values(conn.Do("EXEC")); err != nil {
//		return 0, 0, err
//	} else {
//		lastTstamp, _ := redis.Int64(val[1], nil)
//		currentTokenCount, _ := redis.Int64(val[2], nil)
//		secondsSinceLastRequest := now - lastTstamp
//		addedTokensSinceLastRequest := secondsSinceLastRequest * t.requestsPerSecond
//
//		if currentTokenCount + addedTokensSinceLastRequest > t.burstSize {
//			addedTokensSinceLastRequest = t.burstSize - currentTokenCount
//		}
//
//		if currentTokenCount + addedTokensSinceLastRequest - 1 <= 0 {
//			return 0, int(t.burstSize), nil
//		}
//
//		val2, err := redis.Int(conn.Do("INCRBY", key, addedTokensSinceLastRequest - 1))
//		if err != nil {
//			return 0, 0, err
//		}
//
//		return val2, int(t.burstSize), nil
//	}
//}

func (t *RedisSimpleRateThrottler) DecorateHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user := t.identifyClient(req)
		remaining, limit, err := t.takeToken(user)

		if err != nil {
			t.logger.Error("Error occurred while handling request from %s: %s", req.RemoteAddr, err)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(503)
			rw.Write([]byte("{\"msg\":\"service unavailable\"}"))
			return
		}

		rw.Header().Add("X-RateLimit", strconv.Itoa(limit))
		rw.Header().Add("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if remaining <= 0 {
			t.logger.Notice("Client %s exceeded rate limit", req.RemoteAddr)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(429)
			rw.Write([]byte("{\"msg\":\"rate limit exceeded\"}"))
		} else {
			handler.ServeHTTP(rw, req)
		}
	})
}
