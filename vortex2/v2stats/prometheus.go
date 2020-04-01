package v2stats

import (
	"math"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

type Counter struct {
	prometheus.Counter
}

type Gauge struct {
	prometheus.Gauge

	gauges *prometheus.GaugeVec
	labels prometheus.Labels
}

func (g Gauge) Set(val float64) {
	if math.IsNaN(val) {
		g.Clear()
	} else {
		g.Gauge.Set(val)
	}
}

func (g Gauge) Clear() {
	g.gauges.Delete(g.labels)
}

// Generic interface for gauge metric definition.
type GaugeDefinition struct {
	tagNames []string
	gauges   *prometheus.GaugeVec
}

func (g *GaugeDefinition) V(kvs ...KV) (Gauge, error) {
	labels := make(prometheus.Labels)
	for _, tag := range g.tagNames {
		for _, kv := range kvs {
			if val, ok := kv[tag]; ok {
				labels[tag] = val
			}
		}
	}

	m, err := g.gauges.GetMetricWith(labels)
	if err != nil {
		return Gauge{}, err
	}
	return Gauge{
		Gauge:  m,
		gauges: g.gauges,
		labels: labels,
	}, nil
}

func (g *GaugeDefinition) Must(kvs ...KV) Gauge {
	m, err := g.V(kvs...)
	if err != nil {
		panic(err)
	}
	return m
}

// Generic interface for counter metric definition.
type CounterDefinition struct {
	tagNames []string
	counters *prometheus.CounterVec
}

func (c *CounterDefinition) V(kvs KV) (Counter, error) {
	labels := make(prometheus.Labels)
	for _, tag := range c.tagNames {
		if val, ok := kvs[tag]; ok {
			labels[tag] = val
		}
	}

	if m, err := c.counters.GetMetricWith(labels); err != nil {
		return Counter{}, err
	} else {
		return Counter{m}, nil
	}
}

func (c *CounterDefinition) Must(kvs KV) Counter {
	m, err := c.V(kvs)
	if err != nil {
		panic(err)
	}
	return m
}

func DefineGauge(name string, tagNames ...string) (GaugeDefinition, error) {
	name = strings.Replace(name, "/", ":", -1)
	opts := prometheus.GaugeOpts{
		Name: name,
		Help: "TODO",
	}

	gauges := prometheus.NewGaugeVec(opts, tagNames)
	prometheus.MustRegister(gauges)

	return GaugeDefinition{
		tagNames: tagNames,
		gauges:   gauges,
	}, nil
}

func MustDefineGauge(name string, tagNames ...string) GaugeDefinition {
	m, err := DefineGauge(name, tagNames...)
	if err != nil {
		panic(err)
	}

	return m
}

func DefineGaugeWithHierarchy(
	name string,
	tagNamesHierarchy ...[]string) (GaugeDefinition, error) {

	var tagNames []string
	for _, tags := range tagNamesHierarchy {
		tagNames = append(tagNames, tags...)
	}
	return DefineGauge(name, tagNames...)
}

func DefineCounter(name string, tagNames ...string) (CounterDefinition, error) {
	name = strings.Replace(name, "/", ":", -1)
	opts := prometheus.CounterOpts{
		Name: name,
		Help: "TODO",
	}

	counters := prometheus.NewCounterVec(opts, tagNames)
	prometheus.MustRegister(counters)

	return CounterDefinition{
		tagNames: tagNames,
		counters: counters,
	}, nil
}

func MustDefineCounter(name string, tagNames ...string) CounterDefinition {
	m, err := DefineCounter(name, tagNames...)
	if err != nil {
		panic(err)
	}

	return m
}
