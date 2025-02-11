// Copyright 2022 The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package purefareceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/purefareceiver"

import (
	"context"

	"github.com/prometheus/prometheus/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/purefareceiver/internal"
)

var _ component.MetricsReceiver = (*purefaReceiver)(nil)

type purefaReceiver struct {
	cfg  *Config
	set  component.ReceiverCreateSettings
	next consumer.Metrics

	wrapped component.MetricsReceiver
}

func newReceiver(cfg *Config, set component.ReceiverCreateSettings, next consumer.Metrics) *purefaReceiver {
	return &purefaReceiver{
		cfg:  cfg,
		set:  set,
		next: next,
	}
}

func (r *purefaReceiver) Start(ctx context.Context, compHost component.Host) error {
	fact := prometheusreceiver.NewFactory()
	scrapeCfgs := []*config.ScrapeConfig{}

	arrScraper := internal.NewScraper(ctx, internal.ScraperTypeArray, r.cfg.Endpoint, r.cfg.Arrays, r.cfg.Settings.ReloadIntervals.Array)
	if scCfgs, err := arrScraper.ToPrometheusReceiverConfig(compHost, fact); err == nil {
		scrapeCfgs = append(scrapeCfgs, scCfgs...)
	} else {
		return err
	}

	hostScraper := internal.NewScraper(ctx, internal.ScraperTypeHost, r.cfg.Endpoint, r.cfg.Hosts, r.cfg.Settings.ReloadIntervals.Host)
	if scCfgs, err := hostScraper.ToPrometheusReceiverConfig(compHost, fact); err == nil {
		scrapeCfgs = append(scrapeCfgs, scCfgs...)
	} else {
		return err
	}

	promRecvCfg := fact.CreateDefaultConfig().(*prometheusreceiver.Config)
	promRecvCfg.PrometheusConfig = &config.Config{ScrapeConfigs: scrapeCfgs}

	wrapped, err := fact.CreateMetricsReceiver(ctx, r.set, promRecvCfg, r.next)
	if err != nil {
		return err
	}
	r.wrapped = wrapped

	err = r.wrapped.Start(ctx, compHost)
	if err != nil {
		return err
	}

	return nil
}

func (r *purefaReceiver) Shutdown(ctx context.Context) error {
	if r.wrapped != nil {
		return r.wrapped.Shutdown(ctx)
	}
	return nil
}
