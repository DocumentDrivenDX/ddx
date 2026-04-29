package config

// Clone returns a deep copy of a so that the returned value shares no
// maps, slices, or nested pointer storage with the receiver. Returns
// nil when a is nil.
func (a *AgentConfig) Clone() *AgentConfig {
	if a == nil {
		return nil
	}
	out := *a
	if a.Models != nil {
		out.Models = make(map[string]string, len(a.Models))
		for k, v := range a.Models {
			out.Models[k] = v
		}
	}
	if a.ReasoningLevels != nil {
		out.ReasoningLevels = make(map[string][]string, len(a.ReasoningLevels))
		for k, v := range a.ReasoningLevels {
			out.ReasoningLevels[k] = append([]string(nil), v...)
		}
	}
	if a.Endpoints != nil {
		out.Endpoints = append([]AgentEndpoint(nil), a.Endpoints...)
	}
	out.Routing = a.Routing.Clone()
	if a.Virtual != nil {
		v := *a.Virtual
		if a.Virtual.Normalize != nil {
			v.Normalize = append([]NormalizePattern(nil), a.Virtual.Normalize...)
		}
		out.Virtual = &v
	}
	return &out
}

// Clone returns a deep copy of r. Returns nil when r is nil.
func (r *RoutingConfig) Clone() *RoutingConfig {
	if r == nil {
		return nil
	}
	out := *r
	if r.ProfilePriority != nil {
		out.ProfilePriority = append([]string(nil), r.ProfilePriority...)
	}
	return &out
}

// Clone returns a deep copy of e. Returns nil when e is nil. The
// PerHarness map and each *EvidenceCapsOverride value are duplicated
// so the returned struct shares no pointer storage with e.
func (e *EvidenceCapsConfig) Clone() *EvidenceCapsConfig {
	if e == nil {
		return nil
	}
	out := *e
	out.MaxPromptBytes = clonePtrInt(e.MaxPromptBytes)
	out.MaxInlinedFileBytes = clonePtrInt(e.MaxInlinedFileBytes)
	out.MaxDiffBytes = clonePtrInt(e.MaxDiffBytes)
	out.MaxGoverningDocBytes = clonePtrInt(e.MaxGoverningDocBytes)
	if e.PerHarness != nil {
		out.PerHarness = make(map[string]*EvidenceCapsOverride, len(e.PerHarness))
		for name, h := range e.PerHarness {
			if h == nil {
				out.PerHarness[name] = nil
				continue
			}
			cp := &EvidenceCapsOverride{
				MaxPromptBytes:       clonePtrInt(h.MaxPromptBytes),
				MaxInlinedFileBytes:  clonePtrInt(h.MaxInlinedFileBytes),
				MaxDiffBytes:         clonePtrInt(h.MaxDiffBytes),
				MaxGoverningDocBytes: clonePtrInt(h.MaxGoverningDocBytes),
			}
			out.PerHarness[name] = cp
		}
	}
	return &out
}

// Clone returns a deep copy of m. Returns nil when m is nil.
func (m *ExecutionsMirrorConfig) Clone() *ExecutionsMirrorConfig {
	if m == nil {
		return nil
	}
	out := *m
	if m.Include != nil {
		out.Include = append([]string(nil), m.Include...)
	}
	if m.Async != nil {
		v := *m.Async
		out.Async = &v
	}
	return &out
}

// Clone returns a deep copy of w. Returns nil when w is nil.
func (w *WorkersConfig) Clone() *WorkersConfig {
	if w == nil {
		return nil
	}
	out := *w
	out.MaxCount = clonePtrInt(w.MaxCount)
	if w.DefaultSpec != nil {
		spec := *w.DefaultSpec
		out.DefaultSpec = &spec
	}
	return &out
}

func clonePtrInt(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
