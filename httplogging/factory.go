package httplogging

import (
	"fmt"
	"net/http"

	"github.com/mittwald/servicegateway/auth"
	"github.com/mittwald/servicegateway/config"
	"github.com/op/go-logging"
)

type HttpLogger interface {
	Wrap(http.Handler) (http.Handler, error)
}

func LoggerFromConfig(config *config.LoggingConfiguration, logger *logging.Logger, verifier *auth.JwtVerifier) (HttpLogger, error) {
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
