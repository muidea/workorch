package backend

import "encoding/json"

func decodeSnapshotInfo(v any) (*WorkspaceSnapshotInfo, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out WorkspaceSnapshotInfo
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
