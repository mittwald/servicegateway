package httplogging

import (
	"fmt"
	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
	"net/http"
)

type HttpLogger interface {
	Wrap(http.Handler) (http.Handler, error)
}

func LoggerFromConfig(config *config.LoggingConfiguration, logger *logging.Logger, verifier *auth.JWTVerifier) (HttpLogger, error) {
	switch config.Type {
	case "amqp":
		return NewAmqpLoggingBehaviour(config, logger, verifier)
	case "apache":
		return &ApacheLoggingBehaviour{
			Filename: config.Filename,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported logging type: '%s'", config.Type)
	}
}
