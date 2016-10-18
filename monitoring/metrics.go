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

import "github.com/prometheus/client_golang/prometheus"

type PromMetrics struct {
	TotalResponseTimes    *prometheus.SummaryVec
	UpstreamResponseTimes *prometheus.SummaryVec
	Errors                *prometheus.CounterVec
}

func newMetrics() (*PromMetrics, error) {
	p := new(PromMetrics)

	p.TotalResponseTimes = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "servicegateway",
		Subsystem: "proxy",
		Name: "total_times_seconds",
		Help: "HTTP total response times",
	}, []string{"application"})

	p.UpstreamResponseTimes = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "servicegateway",
		Subsystem: "proxy",
		Name: "upstream_times_seconds",
		Help: "HTTP upstream response times",
	}, []string{"application"})

	p.Errors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "servicegateway",
		Subsystem: "proxy",
		Name: "errors",
		Help: "HTTP proxy errors",
	}, []string{"application", "reason"})

	return p, nil
}

func (m *PromMetrics) Init() {
	prometheus.MustRegister(m.TotalResponseTimes)
	prometheus.MustRegister(m.UpstreamResponseTimes)
	prometheus.MustRegister(m.Errors)
}