//go:build !linux

package file

import "fmt"

func openMutationResultStore(string, string, bool) (*mutationResultStore, error) {
	return nil, fmt.Errorf("restricted File result delivery unavailable on this platform")
}
func (*mutationResultStore) close() {}
func (*mutationResultStore) read(string) ([]byte, error) {
	return nil, fmt.Errorf("restricted File result delivery unavailable")
}
func (*mutationResultStore) write(string, []byte) (bool, error) {
	return false, fmt.Errorf("restricted File result delivery unavailable")
}
