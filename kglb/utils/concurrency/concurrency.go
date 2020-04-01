package concurrency

import (
	"context"
	"sync"
)

// CompleteTasks is a blocking call to perform execution of taskFunccallback
// numTasks times by using specific number of workers specified in numWorkers
// argument (0 means unlimited, number of spawn workers will be equal to number
// of numTasks).
func CompleteTasks(
	ctx context.Context,
	numWorkers int,
	numTasks int,
	taskFunc func(workerId, taskId int)) error {

	// nothing to do if there is no any entries to check.
	if numTasks == 0 {
		return nil
	}

	// use as much workers as number of entries when numWorkers is not
	// specified.
	if numTasks < numWorkers || numWorkers == 0 {
		numWorkers = numTasks
	}

	numToCheck := (numTasks + numWorkers - 1) / numWorkers

	// using wait group to track of completed tasks to make whole call of
	// CompleteTasks() blocking until completion of all tasks.
	var waitGroup sync.WaitGroup
	waitGroup.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerId int, wg *sync.WaitGroup) {
			defer wg.Done()

			start := workerId * numToCheck
			end := (workerId + 1) * numToCheck
			if end > numTasks {
				end = numTasks
			}

			for pos := start; pos < end; pos++ {
				// Stop loop if context canceled
				select {
				case <-ctx.Done():
					return
				default:
					taskFunc(workerId, pos)
				}
			}
		}(i, &waitGroup)
	}

	doneChan := make(chan interface{})
	go func(ch chan interface{}, wg *sync.WaitGroup) {
		// Wait for all workers to complete tasks.
		wg.Wait()
		close(ch)
	}(doneChan, &waitGroup)

	select {
	case <-doneChan:
		// From documentation:
		// 		A select blocks until one of its cases can run, then it executes that case.
		// 		It chooses one at random if multiple are ready.
		// Saying that it is possible that all 2 channels are readable.
		// In this case it may also mean that all workers have finished due to canceled context
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	case <-ctx.Done():
		return ctx.Err()
	}

}
