package preparationcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/muidea/workorch/internal/pkg/bootstrapcompiler"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparedBootstrapCompilerVersion = startuppreparation.BootstrapCompilerVersion

// PreparedBootstrapCompilation describes structural compilation only. In
// particular it has no ready/completion/activation permission bit. The owner
// observation and no-Run protocol must be adjudicated independently.
type PreparedBootstrapCompilation struct {
	CompilerVersion string
	SnapshotHash    string
	ManifestHash    string
	BootstrapHash   string
	Bootstrap       runtimecontract.RuntimeBootstrap
}

// CompilePreparedBootstrap accepts detached typed values only. Unlike the
// existing activation port it cannot load owners, materialize configuration,
// select Memory with embedding, refresh catalogs or initialize Runtime.
func CompilePreparedBootstrap(snapshot startuppreparation.ExactConfirmedStartupSnapshot, manifest startuppreparation.Manifest, activation runtimecontract.RuntimeActivationContext) (PreparedBootstrapCompilation, error) {
	if err := manifest.ValidateFor(snapshot); err != nil {
		return PreparedBootstrapCompilation{}, err
	}
	if err := activation.Validate(); err != nil {
		return PreparedBootstrapCompilation{}, fmt.Errorf("%w: invalid detached activation input", common.ErrInvalidArgument)
	}
	if runtimecontract.StableRuntimeProfileContractsHash(activation.ExecutionContract.ProfileContracts) != snapshot.StartupInput.RuntimeProfileContractsHash {
		return PreparedBootstrapCompilation{}, fmt.Errorf("%w: prepared profile differs from exact startup input", common.ErrConflict)
	}
	inventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(snapshot.Candidate, snapshot.StartupInput, snapshot.ActivationAdmission)
	if err != nil {
		return PreparedBootstrapCompilation{}, err
	}
	// No call to RuntimeBootstrap.InitialFacts or any drive/initialize port.
	// Source obligations are compiled from the original sealed candidate,
	// never from a caller's substitute or a regenerated Definition.
	bootstrap, err := bootstrapcompiler.RuntimeBootstrapWithSourceObligations(&bootstrapcompiler.GoalInput{ID: snapshot.Goal.ID, WorkspaceID: snapshot.Goal.WorkspaceID, Text: snapshot.Goal.Text}, &bootstrapcompiler.DoDInput{ID: snapshot.DoD.ID, WorkspaceID: snapshot.DoD.WorkspaceID, Items: snapshot.DoD.Items}, snapshot.StartupInput.DefinitionRevision, snapshot.StartupInput.StartupInputHash, nil, activation.Clone(), inventory)
	if err != nil {
		return PreparedBootstrapCompilation{}, err
	}
	if err := bootstrap.Validate(); err != nil {
		return PreparedBootstrapCompilation{}, err
	}
	encoded, err := json.Marshal(bootstrap)
	if err != nil || len(encoded) > 4*1024*1024 {
		return PreparedBootstrapCompilation{}, fmt.Errorf("%w: prepared bootstrap exceeds byte limit", common.ErrInvalidArgument)
	}
	digest := sha256.Sum256(encoded)
	return PreparedBootstrapCompilation{
		CompilerVersion: PreparedBootstrapCompilerVersion, SnapshotHash: snapshot.SnapshotHash, ManifestHash: manifest.ManifestHash,
		BootstrapHash: "prepared-bootstrap-sha256:" + hex.EncodeToString(digest[:]), Bootstrap: bootstrap.Clone(),
	}, nil
}
