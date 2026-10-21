package startuppreparation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
)

const CheckPolicyVersion = "workorch-startup-check-policy/v1"
const BootstrapCompilerVersion = "workorch-prepared-bootstrap/v1"
const ManifestCheckLimit = 128

type CheckID string

const (
	CheckExactSeal       CheckID = "exact_seal"
	CheckDirectory       CheckID = "directory_boundary"
	CheckManagedConfigs  CheckID = "managed_configuration"
	CheckSoulSelection   CheckID = "soul_selection"
	CheckSkillSelection  CheckID = "skill_selection"
	CheckMCPSelection    CheckID = "mcp_selection"
	CheckMemorySelection CheckID = "memory_selection"
	CheckProviderCache   CheckID = "provider_cache"
	CheckPersistence     CheckID = "persistence_integrity"
	CheckNoRun           CheckID = "first_activation_no_run"
)

// Check declares the fact needed, never that the fact was observed. Selection
// checks concern only the exact profile/configuration's selected resources;
// an empty selection needs its own valid policy/completeness observation and
// does not require installing an otherwise unused capability.
type Check struct {
	ID                   CheckID
	Required             bool
	Owner                string
	RequirementSourceRef string
	SubjectRef           string
	ExpectedContentHash  string
	EvidenceRule         string
	FreshnessRule        string
	NotApplicableReason  string
}

type Manifest struct {
	SchemaVersion string
	PolicyVersion string
	SnapshotHash  string
	Checks        []Check
	ManifestHash  string
}

// CompilePreparationManifest only declares requirements. It does not infer
// current resource availability from the candidate's DECLARED/Available bits,
// provider configuration, registry membership or Runtime prerequisites.
func CompilePreparationManifest(snapshot ExactConfirmedStartupSnapshot) (Manifest, error) {
	if err := snapshot.Validate(); err != nil {
		return Manifest{}, err
	}
	source := snapshot.StartupInput.StartupInputHash
	checks := []Check{
		{ID: CheckExactSeal, Required: true, Owner: "workspace", RequirementSourceRef: source, SubjectRef: snapshot.Candidate.CandidateHash, EvidenceRule: "exact_confirmed_persisted_all_of", FreshnessRule: "owner_revision_and_seal"},
		{ID: CheckDirectory, Required: true, Owner: "file", RequirementSourceRef: source, SubjectRef: snapshot.StartupInput.ResolvedProjectWorkspaceDirectory, EvidenceRule: "existing_directory_permissions_scope_no_symlink", FreshnessRule: "bounded_owner_observation"},
		{ID: CheckManagedConfigs, Required: true, Owner: "file", RequirementSourceRef: snapshot.AuthoringConfigurationHash, SubjectRef: snapshot.InternalDirectory, EvidenceRule: "existing_exact_managed_configuration_read_only", FreshnessRule: "current_content_and_scope"},
		{ID: CheckSoulSelection, Required: true, Owner: "soul", RequirementSourceRef: snapshot.AuthoringConfigurationHash, SubjectRef: source, EvidenceRule: "exact_selected_souls_current_and_complete", FreshnessRule: "owner_revision_and_scope"},
		{ID: CheckSkillSelection, Required: true, Owner: "skill", RequirementSourceRef: snapshot.AuthoringConfigurationHash, SubjectRef: source, EvidenceRule: "exact_selected_skills_and_complete_catalog", FreshnessRule: "owner_revision_and_scope"},
		{ID: CheckMCPSelection, Required: true, Owner: "mcpregistry", RequirementSourceRef: snapshot.AuthoringConfigurationHash, SubjectRef: source, EvidenceRule: "exact_selection_and_required_connection_facts_no_probe", FreshnessRule: "registry_revision_and_connection_epoch"},
		{ID: CheckMemorySelection, Required: true, Owner: "memory", RequirementSourceRef: snapshot.AuthoringConfigurationHash, SubjectRef: source, EvidenceRule: "original_mode_inventory_and_required_selection_no_embedding", FreshnessRule: "owner_revision_and_selection_scope"},
		{ID: CheckProviderCache, Required: true, Owner: "llm", RequirementSourceRef: snapshot.AuthoringConfigurationHash, SubjectRef: source, EvidenceRule: "cache_only_configuration_capability_no_refresh", FreshnessRule: "owner_configuration_revision_and_cache_expiry"},
		{ID: CheckPersistence, Required: true, Owner: "workspace", RequirementSourceRef: source, SubjectRef: string(snapshot.WorkspaceID), EvidenceRule: "complete_persisted_owner_binding", FreshnessRule: "owner_revision_and_seal"},
		{ID: CheckNoRun, Required: true, Owner: "run", RequirementSourceRef: source, SubjectRef: string(snapshot.WorkspaceID), EvidenceRule: "complete_ordered_no_history_and_workspace_reservation_fence", FreshnessRule: "same_creation_preparation_epoch_and_final_cas"},
	}
	for _, resource := range snapshot.Candidate.Specification.Resources {
		check := Check{
			ID: CheckID("resource/" + resource.ID), Required: resource.Required, Owner: "file",
			RequirementSourceRef: snapshot.AuthoringRef.SpecificationHash, SubjectRef: resource.Ref,
			ExpectedContentHash: resource.SnapshotHash, EvidenceRule: "actual_current_resource_content_and_scope",
			FreshnessRule: "current_content_and_scope",
		}
		if !resource.Required {
			check.NotApplicableReason = "not_required_by_confirmed_specification"
		}
		checks = append(checks, check)
	}
	if len(checks) > ManifestCheckLimit {
		return Manifest{}, fmt.Errorf("%w: preparation manifest exceeds check limit", common.ErrInvalidArgument)
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	for index, check := range checks {
		if check.SubjectRef == "" || index > 0 && check.ID == checks[index-1].ID {
			return Manifest{}, common.ErrConflict
		}
	}
	manifest := Manifest{SchemaVersion: SchemaVersion, PolicyVersion: CheckPolicyVersion, SnapshotHash: snapshot.SnapshotHash, Checks: checks}
	manifest.ManifestHash = manifest.contentHash()
	if manifest.ManifestHash == "" {
		return Manifest{}, fmt.Errorf("%w: preparation manifest exceeds byte limit", common.ErrInvalidArgument)
	}
	return manifest, nil
}

func (m Manifest) Clone() Manifest {
	m.Checks = slices.Clone(m.Checks)
	return m
}

func (m Manifest) ValidateFor(snapshot ExactConfirmedStartupSnapshot) error {
	expected, err := CompilePreparationManifest(snapshot)
	if err != nil {
		return err
	}
	// Full declarations must match; a self-consistent hash cannot excuse
	// omitted required checks, a substituted owner or a fabricated N/A rule.
	if !reflect.DeepEqual(m, expected) {
		return fmt.Errorf("%w: preparation manifest differs from sealed requirements", common.ErrConflict)
	}
	return nil
}

func (m Manifest) contentHash() string {
	m.ManifestHash = ""
	encoded, err := json.Marshal(m)
	if err != nil || len(encoded) > 256*1024 {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return "startup-manifest-sha256:" + hex.EncodeToString(digest[:])
}
