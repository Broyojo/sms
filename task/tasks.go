package task

import (
	"sync"
)

type Task func() error

func Run(tasks []Task, threads int) []error {
	input := make(chan Task)
	go func() {
		for _, task := range tasks {
			input <- task
		}
		close(input)
	}()
	var wg sync.WaitGroup
	results := make(chan error)
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range input {
				results <- task()
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	var errors []error
	for e := range results {
		if e != nil {
			errors = append(errors, e)
		}
	}
	return errors
}
