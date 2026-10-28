//go:build !linux

package file

import "github.com/muidea/workorch/internal/pkg/common"

func createPreparationSkillDirectory(string) error         { return common.ErrConflict }
func createPreparationConfig(string, string, []byte) error { return common.ErrConflict }

func validatePreparationInitializationRoots(...string) error { return common.ErrConflict }
