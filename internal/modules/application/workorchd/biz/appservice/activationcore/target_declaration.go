package activationcore

import "errors"

// ErrTargetDeclarationRequired is returned at activation before a Run exists
// when a direct-evidence profile has no explicit Workspace-owner target.
var ErrTargetDeclarationRequired = errors.New("workspace target declaration required")
