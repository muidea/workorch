package workorchmanagementcontract

// BuiltinDefault is a read-only deployment observation. The resource owner
// independently rechecks both content hashes before any explicit upgrade.
type BuiltinDefault struct {
	Kind                   string `json:"kind"`
	ID                     string `json:"id"`
	Present                bool   `json:"present"`
	CurrentContentHash     string `json:"currentContentHash,omitempty"`
	CurrentBuiltinRevision string `json:"currentBuiltinRevision,omitempty"`
	BuiltinContentHash     string `json:"builtinContentHash"`
	UpgradeAvailable       bool   `json:"upgradeAvailable"`
}

type BuiltinDefaultUpgrade struct {
	Kind                string `json:"kind"`
	ID                  string `json:"id"`
	ExpectedContentHash string `json:"expectedContentHash"`
	TargetContentHash   string `json:"targetContentHash"`
}

type BuiltinDefaultUpgradeResult struct {
	Default  BuiltinDefault `json:"default"`
	Upgraded bool           `json:"upgraded"`
}
