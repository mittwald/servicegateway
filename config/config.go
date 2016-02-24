package config
import "fmt"

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

type Configuration struct {
	Applications   map[string]Application `json:"applications"`
	RateLimiting   RateLimiting           `json:"rate_limiting"`
	Authentication GlobalAuth             `json:"authentication"`
	Consul         ConsulConfiguration    `json:"consul"`
	Http           HttpConfiguration      `json:"http"`
	Proxy          ProxyConfiguration     `json:"proxy"`
	Redis          RedisConfiguration     `json:"redis"`
}

type Application struct {
	Routing      Routing         `json:"routing"`
	Backend      Backend         `json:"backend"`
	Auth         ApplicationAuth `json:"auth"`
	Caching      Caching         `json:"caching"`
	RateLimiting bool            `json:"rate_limiting"`
}

type Routing struct {
	Type     string            `json:"type"`
	Path     string            `json:"path"`
	Patterns map[string]string `json:"patterns"`
	Hostname string            `json:"hostname"`
}

type Backend struct {
	Url      string `json:"url"`
	Service  string `json:"service"`
	Tag      string `json:"tag"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type RedisConfiguration struct {
	Address string `json:"address"`
	Database int `json:"database"`
}

type ApplicationAuth struct {
	Disable bool              `json:"disable"`
	Writer  AuthWriterConfig  `json:"writer"`
}

type GlobalAuth struct {
	Mode               string              `json:"mode"`
//	StorageConfig      StorageAuthConfig   `json:"storage"`
	GraphicalConfig    GraphicalAuthConfig `json:"graphical"`
	ProviderConfig     ProviderAuthConfig  `json:"provider"`
	VerificationKey    []byte              `json:"verification_key"`
	VerificationKeyUrl string              `json:"verification_key_url"`
	KeyCacheTtl        string              `json:"key_cache_ttl"`
}

type ConsulConfiguration struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	DataCenter string `json:"datacenter"`
}

func (c ConsulConfiguration) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type HttpConfiguration struct {
	SetHeaders map[string]string `json:"set_headers"`
}

type ProxyConfiguration struct {
	StripResponseHeaders map[string]bool `json:"strip_res_headers"`
	SetRequestHeaders map[string]string `json:"set_req_headers"`
}

type AuthWriterConfig struct {
	Mode string `json:"mode"`
	Name string `json:"name"`
}

//type StorageAuthConfig struct {
//	Mode           string `json:"mode"`
//	Name           string `json:"name"`
//	CookieDomain   string `json:"cookie_domain"`
//	CookieHttpOnly bool   `json:"cookie_httponly"`
//	CookieSecure   bool   `json:"cookie_secure"`
//}

type GraphicalAuthConfig struct {
	LoginRoute string `json:"login_route"`
}

type ProviderAuthConfig struct {
	Url        string                 `json:"url"`
	Parameters map[string]interface{} `json:"parameters"`
}

type Caching struct {
	Enabled   bool `json:"enabled"`
	Ttl       int  `json:"ttl"`
	AutoFlush bool `json:"auto_flush"`
}

type RateLimiting struct {
	Burst             int    `json:"burst"`
	Window            string `json:"window"`
	RequestsPerSecond int    `json:"requests_per_second"`
}
