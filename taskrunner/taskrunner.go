package taskrunner

import (
	"fmt"
	"sync"
)

// Task represents a single asynchronous task.
type Task struct {
	Run  func() error
	Name string
}

// Runner is responsible for running tasks concurrently.
type Runner struct {
	taskPool TaskPool
}

// TaskPool is an interface for a pool of workers.
type TaskPool interface {
	Submit(task func())
	StopWait()
}

// NewRunner creates a new Runner.
func NewRunner(taskPool TaskPool) *Runner {
	return &Runner{
		taskPool: taskPool,
	}
}

// RunTasks executes a slice of tasks concurrently and returns a TaskResult.
func (r *Runner) RunTasks(tasks []Task) []error {
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errors []error
	)

	wg.Add(len(tasks))

	for _, task := range tasks {
		taskCopy := task // Create a copy to avoid data race

		r.taskPool.Submit(func() {
			defer wg.Done()

			if err := taskCopy.Run(); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("%s: %w", taskCopy.Name, err))
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	return errors
}

func (r *Runner) Close() {
	r.taskPool.StopWait()
}
