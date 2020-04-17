package ratelimit

/*
 * Microservice gateway application
 * Copyright (C) 2015  Martin Helmich <m.helmich@mittwald.de>
 *                     Mittwald CM Service GmbH & Co. KG
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

import (
	"github.com/garyburd/redigo/redis"
	"github.com/julienschmidt/httprouter"
	"github.com/mittwald/servicegateway/config"
	logging "github.com/op/go-logging"
	"github.com/pkg/errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Bucket struct {
	sync.Mutex
	limit     int
	fillLevel int
}

type RateLimitingMiddleware interface {
	DecorateHandler(handler httprouter.Handle) httprouter.Handle
}

type RedisSimpleRateThrottler struct {
	burstSize int64
	window    time.Duration
	redisPool *redis.Pool
	logger    *logging.Logger
}

func NewRateLimiter(cfg config.RateLimiting, red *redis.Pool, logger *logging.Logger) (RateLimitingMiddleware, error) {
	t := new(RedisSimpleRateThrottler)
	t.burstSize = int64(cfg.Burst)
	t.redisPool = red
	t.logger = logger

	if w, err := time.ParseDuration(cfg.Window); err != nil {
		return nil, errors.WithStack(err)
	} else {
		t.window = w
	}

	logger.Infof("Initialize rate limiter (burst size %d)", t.burstSize)

	return t, nil
}

func (t *RedisSimpleRateThrottler) identifyClient(req *http.Request) string {
	auth := req.Header.Get("Authorization")
	if auth != "" {
		return strings.Replace(auth, " ", "", -1)
	}

	addr, _ := net.ResolveTCPAddr("tcp", req.RemoteAddr)
	return addr.IP.String()
}

func (t *RedisSimpleRateThrottler) takeToken(user string) (int, int, error) {
	key := "RL_BUCKET_" + user
	conn := t.redisPool.Get()
	defer func() {
		_ = conn.Close()
	}()

	err := conn.Send("MULTI")
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	err = conn.Send("SET", key, t.burstSize, "EX", t.window.Seconds(), "NX")
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	err = conn.Send("DECR", key)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}

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
//		return 0, 0, errors.WithStack(err)
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
//			return 0, 0, errors.WithStack(err)
//		}
//
//		return val2, int(t.burstSize), nil
//	}
//}

func (t *RedisSimpleRateThrottler) DecorateHandler(handler httprouter.Handle) httprouter.Handle {
	return func(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {
		user := t.identifyClient(req)
		remaining, limit, err := t.takeToken(user)

		if err != nil {
			t.logger.Errorf("Error occurred while handling request from %s: %s", req.RemoteAddr, err)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(503)
			_, _ = rw.Write([]byte("{\"msg\":\"service unavailable\"}"))
			return
		}

		if remaining < 0 {
			remaining = 0
		}

		rw.Header().Add("X-RateLimit", strconv.Itoa(limit))
		rw.Header().Add("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if remaining <= 0 {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(429)
			_, _ = rw.Write([]byte("{\"msg\":\"rate limit exceeded\"}"))
		} else {
			handler(rw, req, p)
		}
	}
}
