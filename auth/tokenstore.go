package auth
import (
	"github.com/garyburd/redigo/redis"
	"crypto/rand"
	"encoding/base64"
	"github.com/hashicorp/golang-lru"
	"fmt"
)

type Token string

type TokenStore interface {
	AddTokenToStore(string) (Token, error)
	GetTokenFromStore(Token) (string, error)
}

type CacheDecorator struct {
	wrapped TokenStore
	localCache *lru.Cache
}

type RedisTokenStore struct {
	redisPool  *redis.Pool
}

type TokenStoreOptions struct {
	LocalCacheBucketSize int
}

func NewTokenStore(redisPool *redis.Pool, options TokenStoreOptions) (TokenStore, error) {
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
		},
		localCache: cache,
	}, nil
}

func (s *RedisTokenStore) AddTokenToStore(jwt string) (Token, error) {
	randomBytes := make([]byte, 32)

	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	tokenStr := base64.StdEncoding.EncodeToString(randomBytes)

	conn := s.redisPool.Get()
	defer conn.Close()
	_, err = conn.Do("HSET", "access_tokens", tokenStr, jwt)

	if err != nil {
		return "", err
	}

	return Token(tokenStr), nil
}

func (s* RedisTokenStore) GetTokenFromStore(token Token) (string, error) {
	conn := s.redisPool.Get()
	defer conn.Close()

	jwt, err := redis.String(conn.Do("HGET", "access_tokens", token))
	if err != nil {
		return "", err
	}

	return jwt, nil
}

func (s* CacheDecorator) AddTokenToStore(jwt string) (Token, error) {
	token, err := s.wrapped.AddTokenToStore(jwt)
	if err != nil {
		return "", err
	}

	s.localCache.Add(token, jwt)
	return token, nil
}

func (s* CacheDecorator) GetTokenFromStore(token Token) (string, error) {
	jwt, ok := s.localCache.Get(token)
	if ok {
		switch t := jwt.(type) {
		case string: return t, nil
		default: return "", fmt.Errorf("invalid data type for token %s", token)
		}
	}

	return s.wrapped.GetTokenFromStore(token)
}