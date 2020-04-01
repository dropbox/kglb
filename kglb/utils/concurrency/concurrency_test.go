package concurrency

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type ConcurrencySuite struct {
}

var _ = Suite(&ConcurrencySuite{})

func (m *ConcurrencySuite) TestCompleteTasksSingleWorker(c *C) {
	errChan := make(chan error, 1)
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	go func() {
		err := CompleteTasks(
			ctx,
			1,
			10,
			func(workerId int, taskId int) {
				if workerId > 0 {
					errChan <- fmt.Errorf("unexpected workerId: %d!=0", workerId)
				}
			})
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case err, ok := <-errChan:
		c.Assert(err, NoErr)
		c.Assert(ok, IsFalse)
	case <-time.After(time.Second):
		c.Log("fails to wait completion")
		c.Fail()
	}
}

// Validate distribution of tasks per worker (10 tasks by 2 workers).
func (m *ConcurrencySuite) TestCompleteTasksMultiWorkerEvenTasks(c *C) {
	errChan := make(chan error, 1)
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// keep track of executed tasks by each worker.
	var workerExecs [2]uint32

	go func() {
		err := CompleteTasks(
			ctx,
			2,
			10,
			func(workerId int, taskId int) {
				if workerId > 1 {
					errChan <- fmt.Errorf("unexpected workerId: %d>1", workerId)
				}
				atomic.AddUint32(&workerExecs[workerId], 1)
			})
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case err, ok := <-errChan:
		c.Assert(err, NoErr)
		c.Assert(ok, IsFalse)
	case <-time.After(time.Second):
		c.Log("fails to wait completion")
		c.Fail()
	}
	c.Assert(workerExecs[0], Equals, uint32(5))
	c.Assert(workerExecs[1], Equals, uint32(5))
}

// Validate distribution of tasks per worker (10 tasks by 3 workers).
func (m *ConcurrencySuite) TestCompleteTasksMultiWorkerUnevenTasks(c *C) {
	errChan := make(chan error, 1)
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// keep track of executed tasks by each worker.
	var workerExecs [3]uint32

	go func() {
		err := CompleteTasks(
			ctx,
			3,
			10,
			func(workerId int, taskId int) {
				if workerId > 2 {
					errChan <- fmt.Errorf("unexpected workerId: %d>2", workerId)
				}
				atomic.AddUint32(&workerExecs[workerId], 1)
			})
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case err, ok := <-errChan:
		c.Assert(err, NoErr)
		c.Assert(ok, IsFalse)
	case <-time.After(time.Second):
		c.Log("fails to wait completion")
		c.Fail()
	}
	c.Assert(workerExecs[0], Equals, uint32(4))
	c.Assert(workerExecs[1], Equals, uint32(4))
	c.Assert(workerExecs[2], Equals, uint32(2))
}

func (m *ConcurrencySuite) TestCompleteTasksUnlimited(c *C) {
	errChan := make(chan error, 1)
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	const numTasks = 10
	// keep track of executed tasks by each worker.
	var workerExecs [numTasks]uint32

	go func() {
		err := CompleteTasks(
			ctx,
			0,
			numTasks,
			func(workerId int, taskId int) {
				if workerId > 9 {
					errChan <- fmt.Errorf("unexpected workerId: %d>9", workerId)
				}
				atomic.AddUint32(&workerExecs[workerId], 1)
			})
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case err, ok := <-errChan:
		c.Assert(err, NoErr)
		c.Assert(ok, IsFalse)
	case <-time.After(time.Second):
		c.Log("fails to wait completion")
		c.Fail()
	}

	for i := 0; i < numTasks; i++ {
		c.Assert(workerExecs[i], Equals, uint32(1))
	}
}

func (m *ConcurrencySuite) TestContextDeadline(c *C) {
	errChan := make(chan error, 1)
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelFunc()

	const numTasks = 10
	go func() {
		err := CompleteTasks(
			ctx,
			10,
			numTasks,
			func(workerId int, taskId int) {
				time.Sleep(time.Millisecond * 100)
			})
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	select {
	case err, ok := <-errChan:
		c.Assert(err, ErrorMatches, "context deadline exceeded")
		c.Assert(ok, IsTrue)
	case <-time.After(time.Second):
		c.Log("fails to wait completion")
		c.Fail()
	}
}

func (m *ConcurrencySuite) TestContextCancelled(c *C) {
	errChan := make(chan error)
	ctx, cancelFunc := context.WithCancel(context.Background())

	const numTasks = 10
	go func() {
		err := CompleteTasks(
			ctx,
			numTasks,
			numTasks,
			func(workerId int, taskId int) {
				time.Sleep(time.Second)
			})
		errChan <- err
	}()

	cancelFunc()

	select {
	case err := <-errChan:
		c.Assert(err, ErrorMatches, "context canceled")
	case <-time.After(time.Second):
		c.Log("context is not canceled")
		c.Fail()
	}
}
