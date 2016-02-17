package auth

import (
	"crypto/rand"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/hashicorp/golang-lru"
	"encoding/base32"
)

type MappedToken struct {
	Jwt   string
	Token string
}

type TokenStore interface {
	AddToken(string) (string, error)
	SetToken(string, string) error
	GetToken(string) (string, error)
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

func (s *RedisTokenStore) SetToken(token string, jwt string) (error) {
	var expirationTstamp int64

	valid, claims, err := s.verifier.VerifyToken(jwt)
	if !valid {
		return fmt.Errorf("JWT is invalid")
	}

	if err != nil {
		return fmt.Errorf("bad JWT: %s", err)
	}

	exp, ok := claims["exp"]
	if ok {
		expAsFloat, ok := exp.(float64)
		if !ok {
			return fmt.Errorf("token contained non-number exp time")
		}

		expirationTstamp = int64(expAsFloat)
		fmt.Printf("expiration timestamp: %d", expirationTstamp)
	}

	key := "token_" + token

	conn := s.redisPool.Get()
	defer conn.Close()

	_, err = conn.Do("HMSET", key, "jwt", jwt, "token", token)
	if err != nil {
		return err
	}

	if expirationTstamp > 0 {
		_, err = conn.Do("EXPIREAT", key, expirationTstamp)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *RedisTokenStore) AddToken(jwt string) (string, error) {
	randomBytes := make([]byte, 32)

	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	tokenStr := base32.StdEncoding.EncodeToString(randomBytes)

	err = s.SetToken(tokenStr, jwt)
	if err != nil {
		return "", err
	}

	return tokenStr, nil
}

func (s *RedisTokenStore) GetToken(token string) (string, error) {
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

func (s *CacheDecorator) SetToken(token, jwt string) error {
	err := s.wrapped.SetToken(token, jwt)
	if err != nil {
		return err
	}

	s.localCache.Add(token, jwt)
	return nil
}

func (s *CacheDecorator) AddToken(jwt string) (string, error) {
	token, err := s.wrapped.AddToken(jwt)
	if err != nil {
		return "", err
	}

	s.localCache.Add(token, jwt)
	return token, nil
}

func (s *CacheDecorator) GetToken(token string) (string, error) {
	jwt, ok := s.localCache.Get(token)
	if ok {
		switch t := jwt.(type) {
		case string:
			return t, nil
		default:
			return "", fmt.Errorf("invalid data type for token %s", token)
		}
	}

	return s.wrapped.GetToken(token)
}

func (s *CacheDecorator) GetAllTokens() (<-chan MappedToken, error) {
	return s.wrapped.GetAllTokens()
}
