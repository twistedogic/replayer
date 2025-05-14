package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/alertmanager/timeinterval"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

type Override struct {
	Delta  *int
	Value  *int
	Period timeinterval.TimeInterval
}

func (s Override) update(g prometheus.Gauge) bool {
	if s.Period.ContainsTime(time.Now()) {
		switch {
		case s.Value != nil:
			g.Set(float64(*s.Value))
			return true
		case s.Delta != nil:
			g.Add(float64(*s.Delta))
			return true
		}
	}
	return false
}

type Series struct {
	Initial, Delta int
	Interval       model.Duration
	Labels         map[string]string
	Overrides      []Override
}

func (s Series) updateOnce(v prometheus.Gauge) {
	log.Printf("update metrics %q", v.Desc().String())
	for _, o := range s.Overrides {
		if o.update(v) {
			return
		}
	}
	v.Add(float64(s.Delta))
}

func (s Series) startUpdate(vec *prometheus.GaugeVec) {
	v := vec.With(s.Labels)
	v.Set(float64(s.Initial))
	for range time.Tick(time.Duration(s.Interval)) {
		s.updateOnce(v)
	}
}

type Metrics struct {
	Name   string
	Series []Series
}

func (m *Metrics) labelNames() []string {
	labelMap := make(map[string]struct{})
	for _, s := range m.Series {
		for l := range s.Labels {
			labelMap[l] = struct{}{}
		}
	}
	labels := make([]string, 0, len(labelMap))
	for name := range labelMap {
		labels = append(labels, name)
	}
	return labels
}

func (m *Metrics) Register(reg prometheus.Registerer) error {
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: m.Name,
	}, m.labelNames())
	if err := reg.Register(v); err != nil {
		return err
	}
	for _, s := range m.Series {
		go s.startUpdate(v)
	}
	return nil
}

type Replayer struct {
	Port    int
	Metrics []*Metrics
}

func NewReplayer(path string) (Replayer, error) {
	var r Replayer
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	err = yaml.UnmarshalStrict(b, &r)
	return r, err
}

func (r Replayer) Start() error {
	for _, m := range r.Metrics {
		if err := m.Register(prometheus.DefaultRegisterer); err != nil {
			return err
		}
	}
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(fmt.Sprintf(":%d", r.Port), nil)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		log.Fatal("no config file provided")
	}
	r, err := NewReplayer(args[0])
	if err != nil {
		log.Fatal(err)
	}
	log.Print("starting server at ", r.Port)
	if err := r.Start(); err != nil {
		log.Fatal(err)
	}
}
