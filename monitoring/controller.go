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
	"github.com/hashicorp/consul/api"
	"github.com/op/go-logging"
	"net/http"
	"os"
)

type MonitoringController struct {
	Shutdown         chan bool
	ShutdownComplete chan bool

	httpAddress string
	httpPort    int
	httpServer  *MonitoringServer

	logger *logging.Logger

	promMetrics *PromMetrics

	consulClient    *api.Client
	consulServiceID string
}

func NewMonitoringController(address string, port int, consul *api.Client, logger *logging.Logger) (*MonitoringController, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	server, err := NewMonitoringServer()
	if err != nil {
		return nil, err
	}

	metrics, err := newMetrics()
	if err != nil {
		return nil, err
	}

	return &MonitoringController{
		Shutdown:         make(chan bool),
		ShutdownComplete: make(chan bool),
		httpAddress:      address,
		httpPort:         port,
		httpServer:       server,
		logger:           logger,
		promMetrics:      metrics,
		consulClient:     consul,
		consulServiceID:  fmt.Sprintf("servicegateway-%s", hostname),
	}, nil
}

func (m *MonitoringController) Metrics() *PromMetrics {
	return m.promMetrics
}

func (m *MonitoringController) Start() error {
	m.logger.Info("Registering node in Consul")

	registration := api.AgentServiceRegistration{
		ID:   m.consulServiceID,
		Name: "servicegateway",
		Port: m.httpPort,
		Check: &api.AgentServiceCheck{
			HTTP:     fmt.Sprintf("http://localhost:%d/status", m.httpPort),
			Interval: "30s",
		},
	}

	if err := m.consulClient.Agent().ServiceRegister(&registration); err != nil {
		m.logger.Errorf("Error while registering node in Consul: %s", err)
		return err
	} else {
		m.logger.Info("Successfully registered node in Consul")
	}

	m.promMetrics.Init()

	go func() {
		http.ListenAndServe(fmt.Sprintf("%s:%d", m.httpAddress, m.httpPort), m.httpServer)
	}()

	go func() {
		<-m.Shutdown
		m.shutdown()
	}()

	return nil
}

func (m *MonitoringController) shutdown() error {
	if err := m.consulClient.Agent().ServiceDeregister(m.consulServiceID); err != nil {
		m.logger.Errorf("Error while deregistering service in Consul: %s", err)
		return err
	}

	m.logger.Info("Successfully deregistered service in Consul")
	m.ShutdownComplete <- true

	return nil
}
