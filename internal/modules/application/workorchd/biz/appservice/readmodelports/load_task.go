package readmodelports

import "fmt"

func namedLoadTask(name string, fn func() error) func() error {
	return func() error {
		if fn == nil {
			return nil
		}
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
}
