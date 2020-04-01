package health_manager

import (
	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type StatusCheckerSuite struct {
}

var _ = Suite(&StatusCheckerSuite{})

func (m *StatusCheckerSuite) TestRaisingFalling(c *C) {
	riseCount := 3
	fallCount := 1

	entry := NewHealthStatusEntry(false)
	// check initial healthy status.
	c.Assert(entry.IsHealthy(), IsFalse)

	// need 3 successfull health checks before marking host as healthy.
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsTrue)
	// but single failure to mark it as unhealthy, because of params configured
	// above.
	c.Assert(entry.UpdateHealthCheckStatus(false, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsFalse)
	// repeating test.
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsTrue)
	c.Assert(entry.UpdateHealthCheckStatus(false, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsFalse)

	// the same set of tests, but with initially healthy entry.
	entry = NewHealthStatusEntry(true)
	// check initial healthy status.
	c.Assert(entry.IsHealthy(), IsTrue)

	// need 3 successful health checks before marking host as healthy.
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.IsHealthy(), IsTrue)
	// but single failure to mark it as unhealthy, because of params configured
	// above.
	c.Assert(entry.UpdateHealthCheckStatus(false, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsFalse)
	// repeating test.
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsFalse)
	c.Assert(entry.UpdateHealthCheckStatus(true, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsTrue)
	c.Assert(entry.UpdateHealthCheckStatus(false, riseCount, fallCount), IsTrue)
	c.Assert(entry.IsHealthy(), IsFalse)
}

// Concurrency test of read/update healthStatusEntry.
func (m *StatusCheckerSuite) TestConcurrency(c *C) {
	entry := NewHealthStatusEntry(false)
	// check initial healthy status.
	c.Assert(entry.IsHealthy(), IsFalse)

	stopChan := make(chan interface{}, 20)
	// separate goroutine to read fields.
	go func() {
		for {
			select {
			case _, ok := <-stopChan:
				if ok {
					_ = entry.IsHealthy()
				} else {
					return
				}
			}
		}
	}()

	// main goroutine to update fields.
	for i := 0; i < 20; i++ {
		stopChan <- nil
		originalStatus := entry.IsHealthy()
		entry.UpdateHealthCheckStatus(!originalStatus, 1, 1)
		stopChan <- nil
	}

	close(stopChan)
}
