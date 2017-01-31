package httplogging

import (
	"github.com/gorilla/handlers"
	"net/http"
	"os"
)

type ApacheLoggingBehaviour struct {
	Filename string
}

func (c *ApacheLoggingBehaviour) Wrap(wrapped http.Handler) (http.Handler, error) {
	writer, err := os.OpenFile(c.Filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return handlers.CombinedLoggingHandler(writer, wrapped), nil
}
