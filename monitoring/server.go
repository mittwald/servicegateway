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
	"net/http"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MonitoringServer struct {
}

func NewMonitoringServer() (*MonitoringServer, error) {
	return &MonitoringServer{}, nil
}

func (s *MonitoringServer) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	promHandler := promhttp.Handler()

	mux := httprouter.New()
	mux.GET("/status", func(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		res.Write([]byte("Hallo Welt!"))
	})
	mux.GET("/metrics", func(res http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		promHandler.ServeHTTP(res, req)
	})

	mux.ServeHTTP(res, req)
}