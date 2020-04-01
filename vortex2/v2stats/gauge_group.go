package v2stats

import (
	"math"
	"sync"
)

type gaugeAndValue struct {
	gauge Gauge
	value float64
}

type GaugeGroup struct {
	mu              sync.Mutex
	gaugeDef        GaugeDefinition
	gaugesAndValues map[string]gaugeAndValue
}

func NewGaugeGroup(gaugeDef GaugeDefinition) *GaugeGroup {
	return &GaugeGroup{
		gaugeDef:        gaugeDef,
		gaugesAndValues: make(map[string]gaugeAndValue),
	}
}

// PrepareToSet records that we want to set the kv to value. It does not
// actually set the gauge value until SetAndReset is called.
//
// Note: we don't immediately set the value to ensure that SetAndReset is
// called, since that's required for the metric clearing to work.
func (g *GaugeGroup) PrepareToSet(value float64, kv KV) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	kvStr := kv.String()

	var gauge Gauge
	if gv, ok := g.gaugesAndValues[kvStr]; ok {
		gauge = gv.gauge
	} else {
		var err error
		gauge, err = g.gaugeDef.V(kv)
		if err != nil {
			return err
		}
	}

	g.gaugesAndValues[kvStr] = gaugeAndValue{
		gauge: gauge,
		value: value,
	}
	return nil
}

func (g *GaugeGroup) MustPrepareToSet(value float64, kv KV) {
	g.mu.Lock()
	defer g.mu.Unlock()

	err := g.PrepareToSet(value, kv)
	if err != nil {
		panic(err)
	}
}

// SetAndReset sets all the gauge values and clears values that weren't prepared
// since the last call to SetAndReset.
func (g *GaugeGroup) SetAndReset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Set all the values.
	for _, gv := range g.gaugesAndValues {
		gv.gauge.Set(gv.value)
	}

	// Prepare gauges to be cleared upon the next call to SetAndReset, and
	// delete gauges that were cleared on this call.
	for kvStr, gv := range g.gaugesAndValues {
		if math.IsNaN(gv.value) {
			// Value has already been cleared, no need to store it anymore.
			delete(g.gaugesAndValues, kvStr)
		} else {
			// Value will be cleared next time unless it's set to a non-NaN value.
			gv.gauge.Set(math.NaN())
			g.gaugesAndValues[kvStr] = gaugeAndValue{
				gauge: gv.gauge,
				value: math.NaN(),
			}
		}
	}
}
