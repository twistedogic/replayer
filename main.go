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

type Series struct {
	Delta  int
	Period timeinterval.TimeInterval
}

func (s Series) update(m *Metrics) bool {
	if s.Period.ContainsTime(time.Now()) {
		m.update(s.Delta)
		return true
	}
	return false
}

type Metrics struct {
	Name      string
	Interval  model.Duration
	Delta     int
	Labels    map[string]string
	Initial   int
	Overrides []Series
	metric    *prometheus.GaugeVec
}

func (m *Metrics) Register(reg prometheus.Registerer) error {
	labels := make([]string, 0, len(m.Labels))
	for l := range m.Labels {
		labels = append(labels, l)
	}
	v := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: m.Name,
	}, labels)
	v.With(m.Labels).Set(float64(m.Initial))
	if err := reg.Register(v); err != nil {
		return err
	}
	m.metric = v
	return nil
}

func (m *Metrics) update(delta int) {
	log.Printf("update metrics %q with %d", m.Name, delta)
	m.metric.With(m.Labels).Add(float64(delta))
}

func (m *Metrics) Update() {
	for _, s := range m.Overrides {
		if s.update(m) {
			return
		}
	}
	m.update(m.Delta)
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
		go func() {
			for range time.Tick(time.Duration(m.Interval)) {
				m.Update()
			}
		}()
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
