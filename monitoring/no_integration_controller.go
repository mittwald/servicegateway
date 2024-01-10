package monitoring

/*
 * Microservice gateway application
 * Copyright (C) 2016  Martin Helmich <m.helmich@mittwald.de>
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
	"fmt"

	"github.com/op/go-logging"

	"net/http"
)

type noIntegrationController struct {
	Shutdown         chan bool
	ShutdownComplete chan bool

	httpAddress string
	httpPort    int
	httpServer  *MonitoringServer

	logger *logging.Logger

	promMetrics *PromMetrics
}

func NewNoIntegrationMonitoringController(address string, port int, logger *logging.Logger) (Controller, error) {
	server, err := NewMonitoringServer()
	if err != nil {
		return nil, err
	}

	metrics, err := newMetrics()
	if err != nil {
		return nil, err
	}

	return &noIntegrationController{
		Shutdown:         make(chan bool),
		ShutdownComplete: make(chan bool),
		httpAddress:      address,
		httpPort:         port,
		httpServer:       server,
		logger:           logger,
		promMetrics:      metrics,
	}, nil
}

func (m *noIntegrationController) Metrics() *PromMetrics {
	return m.promMetrics
}

func (m *noIntegrationController) Start() error {
	m.promMetrics.Init()

	go func() {
		err := http.ListenAndServe(fmt.Sprintf("%s:%d", m.httpAddress, m.httpPort), m.httpServer)
		if err != nil {
			m.logger.Error(err)
		}
	}()

	go func() {
		<-m.Shutdown
		err := m.shutdown()
		m.logger.Error(err)
	}()

	return nil
}

func (m *noIntegrationController) shutdown() error {
	m.ShutdownComplete <- true

	return nil
}

func (m *noIntegrationController) SendShutdown() {
	m.Shutdown <- true
}

func (m *noIntegrationController) WaitForShutdown() {
	<-m.ShutdownComplete
}
