package health_manager

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/discovery"
	"dropbox/kglb/utils/health_checker"
	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type fakeResolver struct {
	// update channel.
	updateChan   chan discovery.DiscoveryState
	getStateFunc func() discovery.DiscoveryState
}

func newFakeResolver(getStateFunc func() discovery.DiscoveryState) *fakeResolver {
	return &fakeResolver{
		updateChan:   make(chan discovery.DiscoveryState, 1),
		getStateFunc: getStateFunc,
	}
}

func (f *fakeResolver) GetId() string {
	return "fake"
}

// Get current state of the resolver.
func (f *fakeResolver) GetState() discovery.DiscoveryState {
	return f.getStateFunc()
}

// Get changes of the resolver through channel.
func (f *fakeResolver) Updates() <-chan discovery.DiscoveryState {
	return f.updateChan
}

// Close/Stop resolver.
func (f *fakeResolver) Close() {
}

// Check if the item discovers exactly the same things.
func (f *fakeResolver) Equal(item discovery.DiscoveryResolver) bool {
	return false
}

var _ discovery.DiscoveryResolver = &fakeResolver{}

// mock health checker.
type MockChecker struct {
	checkFunc func(host string, port int) error
}

func (m *MockChecker) Check(host string, port int) error {
	return m.checkFunc(host, port)
}

func (d *MockChecker) GetConfiguration() *hc_pb.HealthCheckerAttributes {
	return nil
}

var _ health_checker.HealthChecker = &MockChecker{}

type HealthManagerSuite struct {
}

var _ = Suite(&HealthManagerSuite{})

func (m *HealthManagerSuite) TestBasic(c *C) {
	// resolver.
	resolver, err := discovery.NewStaticResolver(discovery.StaticResolverParams{
		Id: "resolver",
		Hosts: discovery.DiscoveryState([]*discovery.HostPort{
			discovery.NewHostPort("host1", 80, true),
			discovery.NewHostPort("host2", 80, true),
			// host3 disabled
			discovery.NewHostPort("host3", 80, false),
		}),
	})
	c.Assert(err, NoErr)

	// checker.
	checker, err := health_checker.NewDummyChecker(nil)
	c.Assert(err, NoErr)

	params := HealthManagerParams{
		Id:            c.TestName(),
		Resolver:      resolver,
		HealthChecker: checker,
		UpstreamCheckerAttributes: &hc_pb.UpstreamChecker{
			RiseCount:        1,
			FallCount:        1,
			IntervalMs:       1000,
			ConcurrencyLimit: 1,
		},
	}

	ctx := context.Background()
	mng, err := NewHealthManager(ctx, params)
	c.Assert(err, NoErr)

	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 3)
		c.Assert(state[0].Status.IsHealthy(), IsTrue)
		c.Assert(state[1].Status.IsHealthy(), IsTrue)
		// host3 is disabled, hence unhealthy
		c.Assert(state[2].Status.IsHealthy(), IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait first update")
	}

	// Checking close health checker loop.
	mng.Close()
	select {
	case _, ok := <-mng.updateChan:
		c.Assert(ok, IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait closing state.")
	}
}

func (m *HealthManagerSuite) TestInitialState(c *C) {
	// resolver.
	resolver, err := discovery.NewStaticResolver(discovery.StaticResolverParams{
		Id: "resolver",
		Hosts: discovery.DiscoveryState([]*discovery.HostPort{
			discovery.NewHostPort("host1", 80, true),
			discovery.NewHostPort("host2", 80, true),
		}),
	})
	c.Assert(err, NoErr)

	// checker.
	checker, err := health_checker.NewDummyChecker(nil)
	c.Assert(err, NoErr)

	params := HealthManagerParams{
		Id:            c.TestName(),
		Resolver:      resolver,
		HealthChecker: checker,
		UpstreamCheckerAttributes: &hc_pb.UpstreamChecker{
			RiseCount:  4,
			FallCount:  1,
			IntervalMs: 10,
		},
	}

	ctx := context.Background()
	startTime := time.Now()
	mng, err := NewHealthManager(ctx, params)
	c.Assert(err, NoErr)

	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 2)
		c.Assert(state[0].Status.IsHealthy(), IsFalse)
		c.Assert(state[1].Status.IsHealthy(), IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}

	select {
	case state, ok := <-mng.Updates():
		elapsed := time.Since(startTime)
		// first update should not come early than 30 milliseconds, because of health
		// checking (RiseCount * Interval).
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 2)
		c.Assert(state[0].Status.IsHealthy(), IsTrue)
		c.Assert(state[1].Status.IsHealthy(), IsTrue)
		c.Assert(int(elapsed/time.Millisecond), GreaterThan, 29)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}

	// next update with new entry should come without delays.
	resolver.Update(discovery.DiscoveryState([]*discovery.HostPort{
		// host1 disabled
		discovery.NewHostPort("host1", 80, false),
		discovery.NewHostPort("host2", 80, true),
		discovery.NewHostPort("host3", 80, true),
	}))
	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 3)
		c.Assert(state[0].Status.IsHealthy(), IsFalse)
		c.Assert(state[1].Status.IsHealthy(), IsTrue)
		c.Assert(state[2].Status.IsHealthy(), IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}

	// Checking close health checker loop.
	mng.Close()
	select {
	case _, ok := <-mng.updateChan:
		c.Assert(ok, IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")

	}
}

// Error returned by HealthChecker should be treated as unhealthy result.
func (m *HealthManagerSuite) TestErr(c *C) {
	// resolver.
	resolver, err := discovery.NewStaticResolver(discovery.StaticResolverParams{
		Id: "resolver",
		Hosts: discovery.DiscoveryState([]*discovery.HostPort{
			discovery.NewHostPort("host1", 80, true),
		}),
	})
	c.Assert(err, NoErr)

	// checker.
	var isHealthy uint32 = 1
	checkErr := fmt.Errorf("failed to perform check.")
	checker := &MockChecker{
		checkFunc: func(host string, port int) error {
			if atomic.LoadUint32(&isHealthy) > 0 {
				return nil
			} else {
				return checkErr
			}
		},
	}

	params := HealthManagerParams{
		Id:            c.TestName(),
		Resolver:      resolver,
		HealthChecker: checker,
		UpstreamCheckerAttributes: &hc_pb.UpstreamChecker{
			RiseCount:  4,
			FallCount:  1,
			IntervalMs: 1,
		},
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	mng, err := NewHealthManager(ctx, params)
	c.Assert(err, NoErr)

	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 1)
		c.Assert(state[0].Status.IsHealthy(), IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}

	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 1)
		c.Assert(state[0].Status.IsHealthy(), IsTrue)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")

	}

	// start failing health checks.
	atomic.StoreUint32(&isHealthy, 0)
	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 1)
		c.Assert(state[0].Status.IsHealthy(), IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}
}

func (m *HealthManagerSuite) TestNonBlockingNotify(c *C) {
	// resolver.
	resolver, err := discovery.NewStaticResolver(discovery.StaticResolverParams{
		Id: "resolver",
		Hosts: discovery.DiscoveryState([]*discovery.HostPort{
			discovery.NewHostPort("host1", 80, true),
			discovery.NewHostPort("host2", 80, true),
			discovery.NewHostPort("host3", 80, true),
		}),
	})
	c.Assert(err, NoErr)

	// checker.

	var mapLock sync.RWMutex
	healthMap := make(map[string]interface{})
	checkErr := fmt.Errorf("failed to perform check")

	checker := &MockChecker{
		checkFunc: func(host string, port int) error {
			mapLock.RLock()
			defer mapLock.RUnlock()
			if _, ok := healthMap[host]; ok {
				return nil
			} else {
				return checkErr
			}
		},
	}

	// FIXME(oleg): timeout?
	params := HealthManagerParams{
		Id:            c.TestName(),
		Resolver:      resolver,
		HealthChecker: checker,
		UpstreamCheckerAttributes: &hc_pb.UpstreamChecker{
			RiseCount:  4,
			FallCount:  1,
			IntervalMs: 1,
		},
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	mng, err := NewHealthManager(ctx, params)
	c.Assert(err, NoErr)

	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 3)
		c.Assert(state[0].Status.IsHealthy(), IsFalse)
		c.Assert(state[1].Status.IsHealthy(), IsFalse)
		c.Assert(state[2].Status.IsHealthy(), IsFalse)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}

	// perform some state transitions
	// host1 is up
	mapLock.Lock()
	healthMap["host1"] = struct{}{}
	mapLock.Unlock()
	time.Sleep(time.Millisecond * 100)

	// host2 is up
	mapLock.Lock()
	healthMap["host2"] = struct{}{}
	mapLock.Unlock()
	time.Sleep(time.Millisecond * 100)

	// host3 is up
	mapLock.Lock()
	healthMap["host3"] = struct{}{}
	mapLock.Unlock()
	time.Sleep(time.Millisecond * 100)

	// host1 is down again
	mapLock.Lock()
	delete(healthMap, "host1")
	mapLock.Unlock()

	// skip some updates from HC
	time.Sleep(time.Second * 5)

	// healthManager should return the latest state
	select {
	case state, ok := <-mng.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(len(state), Equals, 3)
		c.Assert(state[0].Status.IsHealthy(), IsFalse)
		c.Assert(state[1].Status.IsHealthy(), IsTrue)
		c.Assert(state[2].Status.IsHealthy(), IsTrue)
	case <-time.After(5 * time.Second):
		c.Fatal("fails to wait update")
	}
}

// validate that health manager properly moves out from initial state even state
// received from resolver is empty.
func (m *HealthManagerSuite) TestInitialEmptyState(c *C) {
	// resolver to simulate empty initial state after timeout.
	fakeResolver := newFakeResolver(
		func() discovery.DiscoveryState {
			return discovery.DiscoveryState{}
		},
	)

	checker, err := health_checker.NewDummyChecker(nil)
	c.Assert(err, NoErr)

	params := HealthManagerParams{
		Id:            c.TestName(),
		Resolver:      fakeResolver,
		HealthChecker: checker,
		UpstreamCheckerAttributes: &hc_pb.UpstreamChecker{
			RiseCount:  4,
			FallCount:  1,
			IntervalMs: 10,
		},
	}

	ctx := context.Background()
	mng, err := NewHealthManager(ctx, params)
	c.Assert(err, NoErr)

	select {
	case <-mng.Updates():
		c.Fatal("unexpected update.")
	case <-time.After(50 * time.Millisecond):
	}

	fakeResolver.updateChan <- discovery.DiscoveryState{}
	select {
	case <-mng.Updates():
	case <-time.After(5 * time.Second):
		c.Fatal("missed update.")
	}
}

func (m *HealthManagerSuite) TestCounterStatMaps(c *C) {
	fakeResolver := newFakeResolver(
		func() discovery.DiscoveryState {
			return discovery.DiscoveryState{}
		},
	)
	checker, err := health_checker.NewDummyChecker(nil)
	c.Assert(err, NoErr)
	params := HealthManagerParams{
		Id:            c.TestName(),
		Resolver:      fakeResolver,
		HealthChecker: checker,
		UpstreamCheckerAttributes: &hc_pb.UpstreamChecker{
			RiseCount:  4,
			FallCount:  1,
			IntervalMs: 10,
		},
	}
	ctx := context.Background()
	mng, err := NewHealthManager(ctx, params)
	c.Assert(err, NoErr)

	// Maps are now empty, create counters
	passCounter1 := mng.getHealthCheckCounter("test", "pass", mng.passCounters)
	failCounter1 := mng.getHealthCheckCounter("test", "fail", mng.failCounters)
	c.Assert(passCounter1, NotNil)
	c.Assert(failCounter1, NotNil)
	// Since source map is different - counters should be unique
	c.Assert(passCounter1, Not(Equals), failCounter1)

	// Ensure that second call returns the same counter
	passCounter2 := mng.getHealthCheckCounter("test", "pass", mng.passCounters)
	failCounter2 := mng.getHealthCheckCounter("test", "fail", mng.failCounters)
	c.Assert(passCounter1, Equals, passCounter2)
	c.Assert(failCounter1, Equals, failCounter2)
}
