package httplogging

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"net/http"
	"strings"
	"time"

	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
	"github.com/streadway/amqp"
)

type AmqpLoggingBehaviour struct {
	Config     *config.LoggingConfiguration
	OnlyUnsafe bool

	logger     *logging.Logger
	connection *amqp.Connection
	channel    *amqp.Channel
	verifier   *auth.JwtVerifier
}

func NewAmqpLoggingBehaviour(cfg *config.LoggingConfiguration, logger *logging.Logger, tokenVerifier *auth.JwtVerifier) (*AmqpLoggingBehaviour, error) {
	c := &AmqpLoggingBehaviour{
		Config:     cfg,
		OnlyUnsafe: cfg.UnsafeOnly,
		logger:     logger,
		verifier:   tokenVerifier,
	}

	err := c.connect()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return c, nil
}

func (c *AmqpLoggingBehaviour) connect() error {
	c.logger.Infof("opening connection to AMQP server: %s", c.Config.Uri)

	conn, err := amqp.Dial(c.Config.Uri)
	if err != nil {
		return fmt.Errorf("error while dialing RabbitMQ: %s", err)
	}

	c.logger.Infof("opened connection to AMQP server: %s", c.Config.Uri)

	closed := make(chan *amqp.Error)
	go func() {
		err := <-closed

		c.logger.Errorf("connection to AMQP server was closed: %s", err)
		c.logger.Noticef("reconnecting after 5 seconds")

		timer := time.NewTimer(5 * time.Second)

		go func() {
			<-timer.C

			c.logger.Errorf("reconnecting after connection error")
			err := c.connect()
			if err != nil {
				c.logger.Errorf("reconnecting after connection error failed")
			}
		}()
	}()

	conn.NotifyClose(closed)

	channel, err := conn.Channel()
	if err != nil {
		return errors.WithStack(err)
	}

	c.connection = conn
	c.channel = channel

	err = channel.ExchangeDeclare(c.Config.Exchange, "topic", true, false, false, false, amqp.Table{})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

type AuditLogAuth struct {
	Sub  string `json:"sub"`
	Sudo string `json:"sudo,omitempty"`
	Ip   string `json:"ip"`
}

type AuditLogMessage struct {
	Auth      AuditLogAuth      `json:"auth"`
	Action    string            `json:"action"`
	Timestamp time.Time         `json:"timestamp"`
	Data      map[string]string `json:"data"`
}

func (c *AmqpLoggingBehaviour) match(req *http.Request) bool {
	if c.OnlyUnsafe {
		return req.Method == "POST" || req.Method == "PATCH" || req.Method == "PUT" || req.Method == "DELETE"
	} else {
		return true
	}
}

func (c *AmqpLoggingBehaviour) OnAuthenticatedRequest(req *http.Request, jwt string) {
	if c.match(req) {
		go func(req *http.Request, jwt string) {
			_, _, mapClaims, err  := c.verifier.VerifyToken(jwt)
			if err != nil {
				c.logger.Errorf("unable to verify token! Message: '%+v'", err)
			}
			var sub string
			var sudo string

			if v, ok := mapClaims["sub"].(string); ok {
				sub = v
			}

			if v, ok := mapClaims["sudo"].(string); ok {
				sudo = v
			}

			entry := AuditLogMessage{
				Auth: AuditLogAuth{
					Sub:  sub,
					Sudo: sudo,
					Ip:   req.RemoteAddr,
				},
				Action:    "api.request." + strings.ToLower(req.Method),
				Timestamp: time.Now(),
				Data: map[string]string{
					"url": req.URL.String(),
				},
			}

			jsonbytes, _ := json.Marshal(&entry)

			key := "api.request." + strings.ToLower(req.Method)
			msg := amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
				ContentType:  "application/json",
				Body:         jsonbytes,
			}

			err = c.channel.Publish(c.Config.Exchange, key, true, false, msg)
			if err != nil {
				c.logger.Errorf("publishing message failed! Message: '%+v'", err)
			}
		}(req, jwt)
	}
}

func (c *AmqpLoggingBehaviour) Wrap(wrapped http.Handler) (http.Handler, error) {
	return wrapped, nil
}
