package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RuntimeResourceDomain is an owner-neutral namespace for mutable facts used
// by Runtime V2 prerequisites. The Orchestrator compares only the canonical
// identity below; it never interprets file paths, repository branches, or
// provider-specific configuration.
type RuntimeResourceDomain string

const (
	RuntimeResourceWorkspaceTree   RuntimeResourceDomain = "workspace_tree"
	RuntimeResourceWorkspaceTarget RuntimeResourceDomain = "workspace_target"
	RuntimeResourceRepositoryState RuntimeResourceDomain = "repository_state"
	RuntimeResourceOwnerCatalog    RuntimeResourceDomain = "owner_catalog"
	RuntimeResourceProviderConfig  RuntimeResourceDomain = "provider_config"
	RuntimeResourceRemoteResource  RuntimeResourceDomain = "remote_resource"
	// RuntimeResourceLocalResource identifies an Orchestrator-local prerequisite
	// resource. It is owner-neutral metadata only: lifecycle probes, CAS and
	// invalidation remain in the Orchestrator SM-2 implementation.
	RuntimeResourceLocalResource RuntimeResourceDomain = "local_resource"
)

func (d RuntimeResourceDomain) Normalized() RuntimeResourceDomain {
	return RuntimeResourceDomain(strings.TrimSpace(string(d)))
}

func ValidRuntimeResourceDomain(domain RuntimeResourceDomain) bool {
	switch domain.Normalized() {
	case RuntimeResourceWorkspaceTree, RuntimeResourceWorkspaceTarget, RuntimeResourceRepositoryState,
		RuntimeResourceOwnerCatalog, RuntimeResourceProviderConfig, RuntimeResourceRemoteResource,
		RuntimeResourceLocalResource:
		return true
	default:
		return false
	}
}

// RuntimeFreshnessDependency identifies the owner-observed revision a
// prerequisite fact depends on. SubjectRef is intentionally exact: owners
// report a workspace_tree change as well as any workspace_target changes when
// both are relevant, instead of asking Orchestrator to infer path ancestry.
type RuntimeFreshnessDependency struct {
	Domain        RuntimeResourceDomain
	OwnerModuleID string
	SubjectRef    string
	Revision      string
}

func (d RuntimeFreshnessDependency) Normalized() RuntimeFreshnessDependency {
	d.Domain = d.Domain.Normalized()
	d.OwnerModuleID = strings.TrimSpace(d.OwnerModuleID)
	d.SubjectRef = strings.TrimSpace(d.SubjectRef)
	d.Revision = strings.TrimSpace(d.Revision)
	return d
}

func (d RuntimeFreshnessDependency) Validate() error {
	d = d.Normalized()
	if !ValidRuntimeResourceDomain(d.Domain) || d.OwnerModuleID == "" || d.SubjectRef == "" || d.Revision == "" {
		return fmt.Errorf("runtime freshness dependency requires domain, owner, subject and revision")
	}
	return nil
}

func runtimeFreshnessDependencyIdentity(value RuntimeFreshnessDependency) string {
	value = value.Normalized()
	return string(value.Domain) + "|" + value.OwnerModuleID + "|" + value.SubjectRef
}

func NormalizeRuntimeFreshnessDependencies(values []RuntimeFreshnessDependency) []RuntimeFreshnessDependency {
	byIdentity := make(map[string]RuntimeFreshnessDependency, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.Domain == "" || value.OwnerModuleID == "" || value.SubjectRef == "" {
			continue
		}
		byIdentity[runtimeFreshnessDependencyIdentity(value)] = value
	}
	keys := make([]string, 0, len(byIdentity))
	for key := range byIdentity {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeFreshnessDependency, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byIdentity[key])
	}
	return nilIfEmpty(ret)
}

// RuntimePrerequisiteObservation is the persisted freshness metadata for a
// satisfied mutable prerequisite. It augments, and never replaces, the typed
// manifest/snapshot/capability/admissibility fact held in RuntimeResourceFacts.
type RuntimePrerequisiteObservation struct {
	PrerequisiteID string
	Kind           PrerequisiteKind
	OwnerModuleID  string
	ObservationID  string
	ContentHash    string
	Dependencies   []RuntimeFreshnessDependency
	ObservedAt     string
}

func (o RuntimePrerequisiteObservation) Normalized() RuntimePrerequisiteObservation {
	o.PrerequisiteID = strings.TrimSpace(o.PrerequisiteID)
	o.Kind = PrerequisiteKind(strings.TrimSpace(string(o.Kind)))
	o.OwnerModuleID = strings.TrimSpace(o.OwnerModuleID)
	o.ObservationID = strings.TrimSpace(o.ObservationID)
	o.ContentHash = strings.TrimSpace(o.ContentHash)
	o.Dependencies = NormalizeRuntimeFreshnessDependencies(o.Dependencies)
	o.ObservedAt = strings.TrimSpace(o.ObservedAt)
	return o
}

func StableRuntimePrerequisiteObservationID(value RuntimePrerequisiteObservation) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, value.PrerequisiteID)
	writeHashString(h, string(value.Kind))
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.ContentHash)
	for _, dependency := range value.Dependencies {
		writeHashString(h, string(dependency.Domain))
		writeHashString(h, dependency.OwnerModuleID)
		writeHashString(h, dependency.SubjectRef)
		writeHashString(h, dependency.Revision)
	}
	return "prerequisite-observation-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func (o RuntimePrerequisiteObservation) Validate() error {
	o = o.Normalized()
	if o.PrerequisiteID == "" || o.Kind == "" || o.OwnerModuleID == "" || o.ContentHash == "" || len(o.Dependencies) == 0 {
		return fmt.Errorf("runtime prerequisite observation requires identity, owner, content and dependencies")
	}
	if o.ObservationID != StableRuntimePrerequisiteObservationID(o) {
		return fmt.Errorf("runtime prerequisite observation identity mismatch")
	}
	for _, dependency := range o.Dependencies {
		if err := dependency.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeRuntimePrerequisiteObservations(values []RuntimePrerequisiteObservation) []RuntimePrerequisiteObservation {
	byPrerequisite := make(map[string]RuntimePrerequisiteObservation, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.PrerequisiteID != "" {
			byPrerequisite[value.PrerequisiteID] = value
		}
	}
	keys := make([]string, 0, len(byPrerequisite))
	for key := range byPrerequisite {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimePrerequisiteObservation, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byPrerequisite[key])
	}
	return nilIfEmpty(ret)
}

func RuntimePrerequisiteObservationForID(values []RuntimePrerequisiteObservation, prerequisiteID string) (RuntimePrerequisiteObservation, bool) {
	prerequisiteID = strings.TrimSpace(prerequisiteID)
	for _, value := range values {
		value = value.Normalized()
		if value.PrerequisiteID == prerequisiteID {
			return value, true
		}
	}
	return RuntimePrerequisiteObservation{}, false
}

// RuntimeResourceChange is a durable owner receipt fact. It identifies a
// concrete resource whose current revision changed during the sealed effect.
type RuntimeResourceChange struct {
	Domain         RuntimeResourceDomain
	OwnerModuleID  string
	SubjectRef     string
	BeforeRevision string
	AfterRevision  string
}

func (c RuntimeResourceChange) Normalized() RuntimeResourceChange {
	c.Domain = c.Domain.Normalized()
	c.OwnerModuleID = strings.TrimSpace(c.OwnerModuleID)
	c.SubjectRef = strings.TrimSpace(c.SubjectRef)
	c.BeforeRevision = strings.TrimSpace(c.BeforeRevision)
	c.AfterRevision = strings.TrimSpace(c.AfterRevision)
	return c
}

func (c RuntimeResourceChange) Validate() error {
	c = c.Normalized()
	if !ValidRuntimeResourceDomain(c.Domain) || c.OwnerModuleID == "" || c.SubjectRef == "" || c.AfterRevision == "" {
		return fmt.Errorf("runtime resource change requires domain, owner, subject and after revision")
	}
	return nil
}

func NormalizeRuntimeResourceChanges(values []RuntimeResourceChange) []RuntimeResourceChange {
	byIdentity := make(map[string]RuntimeResourceChange, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.Domain == "" || value.OwnerModuleID == "" || value.SubjectRef == "" {
			continue
		}
		byIdentity[string(value.Domain)+"|"+value.OwnerModuleID+"|"+value.SubjectRef] = value
	}
	keys := make([]string, 0, len(byIdentity))
	for key := range byIdentity {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeResourceChange, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byIdentity[key])
	}
	return nilIfEmpty(ret)
}

func StableRuntimeResourceChangeInvalidationSignature(receiptHash string, changes []RuntimeResourceChange) string {
	receiptHash = strings.TrimSpace(receiptHash)
	changes = NormalizeRuntimeResourceChanges(changes)
	h := sha256.New()
	writeHashString(h, receiptHash)
	for _, change := range changes {
		writeHashString(h, string(change.Domain))
		writeHashString(h, change.OwnerModuleID)
		writeHashString(h, change.SubjectRef)
		writeHashString(h, change.BeforeRevision)
		writeHashString(h, change.AfterRevision)
	}
	return "resource-change-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimePrerequisiteStale is returned by a read-only owner probe or an
// execution receipt when an observation no longer represents the resource
// immediately before an effect. It is neither a candidate failure nor
// execution evidence.
type RuntimePrerequisiteStale struct {
	PrerequisiteID   string
	ObservationID    string
	Domain           RuntimeResourceDomain
	OwnerModuleID    string
	SubjectRef       string
	ExpectedRevision string
	ObservedRevision string
	Reason           string
}

func (s RuntimePrerequisiteStale) Normalized() RuntimePrerequisiteStale {
	s.PrerequisiteID = strings.TrimSpace(s.PrerequisiteID)
	s.ObservationID = strings.TrimSpace(s.ObservationID)
	s.Domain = s.Domain.Normalized()
	s.OwnerModuleID = strings.TrimSpace(s.OwnerModuleID)
	s.SubjectRef = strings.TrimSpace(s.SubjectRef)
	s.ExpectedRevision = strings.TrimSpace(s.ExpectedRevision)
	s.ObservedRevision = strings.TrimSpace(s.ObservedRevision)
	s.Reason = strings.TrimSpace(s.Reason)
	return s
}

func (s RuntimePrerequisiteStale) Validate() error {
	s = s.Normalized()
	if s.PrerequisiteID == "" || s.ObservationID == "" || !ValidRuntimeResourceDomain(s.Domain) ||
		s.OwnerModuleID == "" || s.SubjectRef == "" || s.ExpectedRevision == "" || s.ObservedRevision == "" || s.Reason == "" {
		return fmt.Errorf("runtime prerequisite stale requires observation identity, resource identity and revisions")
	}
	return nil
}

func StableRuntimePrerequisiteStaleSignature(value RuntimePrerequisiteStale) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, value.PrerequisiteID)
	writeHashString(h, value.ObservationID)
	writeHashString(h, string(value.Domain))
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.SubjectRef)
	writeHashString(h, value.ExpectedRevision)
	writeHashString(h, value.ObservedRevision)
	return "prerequisite-stale-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimePrerequisiteInvalidationRecord is the persisted idempotency fence for
// resource-change/stale processing. It is intentionally diagnostic and does
// not create evidence or a repair directive.
type RuntimePrerequisiteInvalidationRecord struct {
	Signature               string
	PrerequisiteIDs         []string
	ObservationIDs          []string
	InvalidatedDependencies []RuntimeFreshnessDependency
	DispatchID              string
	OperationID             string
	ReceiptHash             string
	// ResourceChanges preserve the owner-observed source of a receipt-driven
	// invalidation for audit/read projections. They do not participate in the
	// reducer beyond the receipt's already-completed exact intersection.
	// Empty is accepted only for pre-field rollout records that retain the
	// receipt hash; new writers must persist the normalized change set.
	ResourceChanges []RuntimeResourceChange
	Stale           *RuntimePrerequisiteStale
}

func (r RuntimePrerequisiteInvalidationRecord) Normalized() RuntimePrerequisiteInvalidationRecord {
	r.Signature = strings.TrimSpace(r.Signature)
	r.PrerequisiteIDs = normalizeStrings(r.PrerequisiteIDs)
	r.ObservationIDs = normalizeStrings(r.ObservationIDs)
	r.InvalidatedDependencies = NormalizeRuntimeFreshnessDependencies(r.InvalidatedDependencies)
	r.DispatchID = strings.TrimSpace(r.DispatchID)
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.ReceiptHash = strings.TrimSpace(r.ReceiptHash)
	r.ResourceChanges = NormalizeRuntimeResourceChanges(r.ResourceChanges)
	if r.Stale != nil {
		stale := r.Stale.Normalized()
		r.Stale = &stale
	}
	return r
}

func (r RuntimePrerequisiteInvalidationRecord) Validate() error {
	r = r.Normalized()
	if r.Signature == "" || len(r.PrerequisiteIDs) == 0 || len(r.ObservationIDs) == 0 {
		return fmt.Errorf("runtime prerequisite invalidation requires signature and affected observations")
	}
	if r.Stale != nil {
		if len(r.ResourceChanges) != 0 {
			return fmt.Errorf("runtime stale invalidation cannot include resource changes")
		}
		return r.Stale.Validate()
	}
	for _, dependency := range r.InvalidatedDependencies {
		if err := dependency.Validate(); err != nil {
			return err
		}
	}
	if r.ReceiptHash == "" {
		return fmt.Errorf("runtime resource-change invalidation requires receipt hash")
	}
	for _, change := range r.ResourceChanges {
		if err := change.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeRuntimePrerequisiteInvalidations(values []RuntimePrerequisiteInvalidationRecord) []RuntimePrerequisiteInvalidationRecord {
	bySignature := make(map[string]RuntimePrerequisiteInvalidationRecord, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.Signature != "" {
			bySignature[value.Signature] = value
		}
	}
	keys := make([]string, 0, len(bySignature))
	for key := range bySignature {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimePrerequisiteInvalidationRecord, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, bySignature[key])
	}
	return nilIfEmpty(ret)
}

// RuntimePrerequisiteInvalidationGeneration returns the exact persisted
// invalidation generation for one prerequisite. Both an owner-confirmed stale
// fact and an accepted durable receipt require a new acquisition generation;
// neither provider/CAS/delivery recovery nor invalidations of unrelated
// prerequisites may change this value.
func RuntimePrerequisiteInvalidationGeneration(values []RuntimePrerequisiteInvalidationRecord, prerequisiteID string) int {
	prerequisiteID = strings.TrimSpace(prerequisiteID)
	if prerequisiteID == "" {
		return 0
	}
	generation := 0
	for _, value := range NormalizeRuntimePrerequisiteInvalidations(values) {
		for _, affectedID := range value.PrerequisiteIDs {
			if affectedID == prerequisiteID {
				generation++
				break
			}
		}
	}
	return generation
}

func RuntimeFreshnessDependencyIntersectsChange(dependency RuntimeFreshnessDependency, change RuntimeResourceChange) bool {
	dependency = dependency.Normalized()
	change = change.Normalized()
	return dependency.Domain == change.Domain && dependency.OwnerModuleID == change.OwnerModuleID && dependency.SubjectRef == change.SubjectRef
}

// StableRuntimePrerequisiteFreshnessHash contributes mutable prerequisite
// observations to an authority transaction. Empty observations deliberately
// hash to the empty string so persisted pre-rollout aggregates remain readable
// until their first fresh acquisition.
func StableRuntimePrerequisiteFreshnessHash(values []RuntimePrerequisiteObservation) string {
	values = NormalizeRuntimePrerequisiteObservations(values)
	if len(values) == 0 {
		return ""
	}
	h := sha256.New()
	for _, value := range values {
		writeHashString(h, value.PrerequisiteID)
		writeHashString(h, value.ObservationID)
		writeHashString(h, value.ContentHash)
		for _, dependency := range value.Dependencies {
			writeHashString(h, string(dependency.Domain))
			writeHashString(h, dependency.OwnerModuleID)
			writeHashString(h, dependency.SubjectRef)
			writeHashString(h, dependency.Revision)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
