package workspace

import (
	"reflect"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

const PreparationInitializationRegistrationVersion = "workorch-preparation-resource-initialization/v2"

type PreparationRegistration struct {
	Plan     skillhubpreparation.RegistrationPlan
	Attempts []PreparationRegistrationAttempt
}
type PreparationRegistrationAttempt struct {
	PreparedAt time.Time
	Query      bool
	Result     *skillhubpreparation.RegistrationObservation
}

func (r *PreparationRegistration) Clone() *PreparationRegistration {
	if r == nil {
		return nil
	}
	out := *r
	out.Attempts = append([]PreparationRegistrationAttempt(nil), r.Attempts...)
	for i := range out.Attempts {
		if out.Attempts[i].Result != nil {
			v := out.Attempts[i].Result.Clone()
			out.Attempts[i].Result = &v
		}
	}
	return &out
}
func (r *PreparationRegistration) Succeeded() bool {
	if r == nil || len(r.Attempts) == 0 {
		return false
	}
	a := r.Attempts[len(r.Attempts)-1]
	if a.Result == nil {
		return false
	}
	receipt, err := a.Result.Receipt(r.Plan, a.Query)
	return err == nil && receipt.Status == "succeeded"
}
func (r *PreparationRegistration) Status() string {
	if r == nil {
		return ""
	}
	if len(r.Attempts) == 0 {
		return "not_started"
	}
	a := r.Attempts[len(r.Attempts)-1]
	if a.Result == nil {
		return "pending"
	}
	receipt, err := a.Result.Receipt(r.Plan, a.Query)
	if err != nil {
		return "unresolved"
	}
	return receipt.Status
}
func (r PreparationResourceInitialization) validateRegistration() error {
	if r.Version == PreparationInitializationVersion {
		if r.Registration != nil {
			return common.ErrConflict
		}
		return nil
	}
	g := r.Registration
	if r.Version != PreparationInitializationRegistrationVersion || g == nil || g.Plan.Validate() != nil || g.Plan.Directory != r.Plan.DeliverableDirectory || g.Plan.RequestID != skillhubpreparation.RegistrationRequestID(string(r.Plan.WorkspaceID), r.Plan.RequestID) || len(g.Attempts) > skillhubpreparation.RegistrationAttemptLimit {
		return common.ErrConflict
	}
	if len(g.Attempts) > 0 && !r.FileObserved() {
		return common.ErrConflict
	}
	at := r.PreparedAt
	if r.FileObserved() {
		at = r.Attempts[len(r.Attempts)-1].Result.CompletedAt
	}
	for i, a := range g.Attempts {
		if a.PreparedAt.Before(at) || a.Query != (i > 0) {
			return common.ErrConflict
		}
		at = a.PreparedAt
		if a.Result != nil {
			if a.Result.Validate() != nil || a.Result.StartedAt.Before(at) {
				return common.ErrConflict
			}
			at = a.Result.CompletedAt
		}
		if i < len(g.Attempts)-1 && a.Result != nil {
			receipt, err := a.Result.Receipt(g.Plan, a.Query)
			if err == nil && (receipt.Status == "succeeded" || receipt.Status == "failed") {
				return common.ErrConflict
			}
		}
	}
	return nil
}
func validateRegistrationProgress(old, next PreparationResourceInitialization) error {
	a, b := old.Registration, next.Registration
	if a == nil || b == nil || !reflect.DeepEqual(old.Attempts, next.Attempts) || !old.FileObserved() || a.Plan != b.Plan || a.Succeeded() {
		return common.ErrConflict
	}
	if len(b.Attempts) == len(a.Attempts)+1 {
		if len(a.Attempts) > 0 {
			last := a.Attempts[len(a.Attempts)-1]
			if last.Result != nil {
				r, e := last.Result.Receipt(a.Plan, last.Query)
				if e == nil && (r.Status == "succeeded" || r.Status == "failed") {
					return common.ErrConflict
				}
			}
		}
		if !reflect.DeepEqual(a.Attempts, b.Attempts[:len(a.Attempts)]) && len(a.Attempts) > 0 || b.Attempts[len(a.Attempts)].Result != nil {
			return common.ErrConflict
		}
		return nil
	}
	if len(a.Attempts) == 0 || len(a.Attempts) != len(b.Attempts) {
		return common.ErrConflict
	}
	i := len(a.Attempts) - 1
	if a.Attempts[i].Result != nil || b.Attempts[i].Result == nil || !reflect.DeepEqual(a.Attempts[:i], b.Attempts[:i]) || a.Attempts[i].PreparedAt != b.Attempts[i].PreparedAt || a.Attempts[i].Query != b.Attempts[i].Query {
		return common.ErrConflict
	}
	return nil
}
