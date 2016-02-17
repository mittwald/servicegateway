package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/hashicorp/golang-lru"
)

type MappedToken struct {
	Jwt   string
	Token string
}

type TokenStore interface {
	AddTokenToStore(string) (string, error)
	GetTokenFromStore(string) (string, error)
	GetAllTokens() (<-chan MappedToken, error)
}

type CacheDecorator struct {
	wrapped    TokenStore
	localCache *lru.Cache
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

func (s *RedisTokenStore) AddTokenToStore(jwt string) (string, error) {
	var expirationTstamp int64

	valid, claims, err := s.verifier.VerifyToken(jwt)
	if !valid {
		return "", fmt.Errorf("JWT is invalid")
	}

	if err != nil {
		return "", fmt.Errorf("bad JWT: %s", err)
	}

	exp, ok := claims["exp"]
	if ok {
		expAsFloat, ok := exp.(float64)
//		fmt.Println(exp)
//		expirationTstamp, ok = exp.(int64)
//		fmt.Printf("expiration timestamp: %d", expirationTstamp)
		if !ok {
			return "", fmt.Errorf("token contained non-number exp time")
		}

		expirationTstamp = int64(expAsFloat)
		fmt.Printf("expiration timestamp: %d", expirationTstamp)
	}

	randomBytes := make([]byte, 32)

	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	tokenStr := base64.StdEncoding.EncodeToString(randomBytes)
	key := "token_" + tokenStr

	conn := s.redisPool.Get()
	defer conn.Close()

	_, err = conn.Do("HMSET", key, "jwt", jwt, "token", tokenStr)
	if err != nil {
		return "", err
	}

	if expirationTstamp > 0 {
		_, err = conn.Do("EXPIREAT", key, expirationTstamp)
		if err != nil {
			return "", err
		}
	}

	return tokenStr, nil
}

func (s *RedisTokenStore) GetTokenFromStore(token string) (string, error) {
	conn := s.redisPool.Get()
	defer conn.Close()

	key := "token_" + token

	jwt, err := redis.String(conn.Do("HGET", key, "jwt"))
	if err == redis.ErrNil {
		return "", NoTokenError
	} else if err != nil {
		return "", err
	}

	return jwt, nil
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

func (s *CacheDecorator) AddTokenToStore(jwt string) (string, error) {
	token, err := s.wrapped.AddTokenToStore(jwt)
	if err != nil {
		return "", err
	}

	s.localCache.Add(token, jwt)
	return token, nil
}

func (s *CacheDecorator) GetTokenFromStore(token string) (string, error) {
	jwt, ok := s.localCache.Get(token)
	if ok {
		switch t := jwt.(type) {
		case string:
			return t, nil
		default:
			return "", fmt.Errorf("invalid data type for token %s", token)
		}
	}

	return s.wrapped.GetTokenFromStore(token)
}

func (s *CacheDecorator) GetAllTokens() (<-chan MappedToken, error) {
	return s.wrapped.GetAllTokens()
}
