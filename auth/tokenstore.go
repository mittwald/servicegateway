package auth

import (
	"crypto/rand"
	"fmt"
	jwt2 "github.com/dgrijalva/jwt-go"
	"github.com/garyburd/redigo/redis"
	"github.com/hashicorp/golang-lru"
	"encoding/base32"
	"strings"
)

type MappedToken struct {
	Jwt   string
	Token string
}

type TokenStore interface {
	AddToken(*JWTResponse) (string, int64, error)
	SetToken(string, *JWTResponse) (int64, error)
	GetToken(string) (*JWTResponse, error)
	GetAllTokens() (<-chan MappedToken, error)
}

type CacheDecorator struct {
	wrapped    TokenStore
	localCache *lru.Cache
}

type CacheRecord struct {
	token *JWTResponse
	exp   int64
}

type RedisTokenStore struct {
	redisPool *redis.Pool
	verifier  *JwtVerifier
}

type TokenStoreOptions struct {
	LocalCacheBucketSize int
}

func NewTokenStore(redisPool *redis.Pool, verifier *JwtVerifier, options TokenStoreOptions) (TokenStore, error) {
	bucketSize := 128

	if options.LocalCacheBucketSize != 0 {
		bucketSize = options.LocalCacheBucketSize
	}

	cache, err := lru.New(bucketSize)
	if err != nil {
		return nil, err
	}

	return &CacheDecorator{
		wrapped: &RedisTokenStore{
			redisPool: redisPool,
			verifier: verifier,
		},
		localCache: cache,
	}, nil
}

func (s *RedisTokenStore) SetToken(token string, jwt *JWTResponse) (int64, error) {
	valid, claims, err := s.verifier.VerifyToken(jwt.JWT)
	if !valid {
		return 0, fmt.Errorf("JWT is invalid")
	}

	if err != nil {
		return 0, fmt.Errorf("bad JWT: %s", err)
	}

	stdClaims, ok := claims.(jwt2.StandardClaims)
	if !ok {
		return 0, fmt.Errorf("error while casting claims")
	}

	key := "token_" + token

	conn := s.redisPool.Get()
	defer conn.Close()

	_, err = conn.Do("HMSET", key, "jwt", jwt.JWT, "token", token, "applications", strings.Join(jwt.AllowedApplications, ";"))
	if err != nil {
		return 0, err
	}

	if stdClaims.ExpiresAt > 0 {
		_, err = conn.Do("EXPIREAT", key, stdClaims.ExpiresAt)
		if err != nil {
			return 0, err
		}
	}

	return stdClaims.ExpiresAt, nil
}

func (s *RedisTokenStore) AddToken(jwt *JWTResponse) (string, int64, error) {
	randomBytes := make([]byte, 32)

	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", 0, err
	}

	tokenStr := base32.StdEncoding.EncodeToString(randomBytes)

	exp, err := s.SetToken(tokenStr, jwt)
	if err != nil {
		return "", 0, err
	}

	return tokenStr, exp, nil
}

func (s *RedisTokenStore) GetToken(token string) (*JWTResponse, error) {
	conn := s.redisPool.Get()
	defer conn.Close()

	key := "token_" + token
	response := JWTResponse{}

	results, err := redis.Strings(conn.Do("HMGET", key, "jwt", "applications"))
	if err == redis.ErrNil {
		return nil, NoTokenError
	} else if err != nil {
		return nil, err
	}

	response.JWT = results[0]
	if results[1] != "" {
		response.AllowedApplications = strings.Split(results[1], ";")
	}

	return &response, nil
}

func (s *RedisTokenStore) GetAllTokens() (<-chan MappedToken, error) {
	conn := s.redisPool.Get()

	keys, err := redis.Strings(conn.Do("KEYS", "token_*"))
	if err != nil {
		conn.Close()
		return nil, err
	}

	c := make(chan MappedToken)
	if len(keys) == 0 {
		close(c)
		conn.Close()
		return c, nil
	}

	go func() {
		for _, key := range keys {
			values, _ := redis.StringMap(conn.Do("HGETALL", key))
			c <- MappedToken{Jwt: values["jwt"], Token: values["token"]}
		}

		conn.Close()
		close(c)
	}()

	return c, nil
}

func (s *CacheDecorator) SetToken(token string, jwt *JWTResponse) (int64, error) {
	exp, err := s.wrapped.SetToken(token, jwt)
	if err != nil {
		return 0, err
	}

	s.localCache.Add(token, &CacheRecord{token: jwt, exp: exp})
	return exp, nil
}

func (s *CacheDecorator) AddToken(jwt *JWTResponse) (string, int64, error) {
	token, exp, err := s.wrapped.AddToken(jwt)
	if err != nil {
		return "", 0, err
	}

	s.localCache.Add(token, &CacheRecord{token: jwt, exp: exp})
	return token, exp, nil
}

func (s *CacheDecorator) GetToken(token string) (*JWTResponse, error) {
	jwt, ok := s.localCache.Get(token)
	if ok {
		switch t := jwt.(type) {
		case string:
			return &JWTResponse{JWT: t}, nil
		case *CacheRecord:
			return t.token, nil
		default:
			return nil, fmt.Errorf("invalid data type for token %s", token)
		}
	}

	return s.wrapped.GetToken(token)
}

func (s *CacheDecorator) GetAllTokens() (<-chan MappedToken, error) {
	return s.wrapped.GetAllTokens()
}
