package readmodelcore

import "sync"

func RunTasks(tasks ...func() error) error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var ret error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if ret == nil {
			ret = err
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			recordErr(task())
		}(task)
	}
	wg.Wait()
	return ret
}
