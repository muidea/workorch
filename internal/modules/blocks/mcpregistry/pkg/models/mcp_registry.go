package mcp

// RegistrySnapshot is the single persistence aggregate for MCP server and
// capability metadata. Revision is a repository CAS fence; consumers receive
// a separately derived content revision from the owner EventHub contract.
type RegistrySnapshot struct {
	Revision     uint64        `json:"revision"`
	Servers      []*Server     `json:"servers,omitempty"`
	Capabilities []*Capability `json:"capabilities,omitempty"`
}

func CloneRegistrySnapshot(snapshot *RegistrySnapshot) *RegistrySnapshot {
	if snapshot == nil {
		return &RegistrySnapshot{}
	}
	return &RegistrySnapshot{
		Revision:     snapshot.Revision,
		Servers:      CloneServers(snapshot.Servers),
		Capabilities: CloneCapabilities(snapshot.Capabilities),
	}
}

func CloneServer(item *Server) *Server {
	if item == nil {
		return nil
	}
	ret := *item
	ret.Args = append([]string(nil), item.Args...)
	ret.Env = cloneStringMap(item.Env)
	ret.AllowedTools = append([]string(nil), item.AllowedTools...)
	ret.AllowedResources = append([]string(nil), item.AllowedResources...)
	ret.AllowedPrompts = append([]string(nil), item.AllowedPrompts...)
	return &ret
}

func CloneServers(items []*Server) []*Server {
	if len(items) == 0 {
		return nil
	}
	ret := make([]*Server, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneServer(item))
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
