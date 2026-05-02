package graphql

// Federation resolvers (B14.6b). The hub fans out a small set of read-only
// queries to every active spoke via the B14.6a fan-out client, decorates
// each row with routing metadata (node_id, project_id, project_url,
// write_capability, status), and merges the per-spoke responses into a
// single result. Local query types (beads, runs, projects) are unchanged —
// these federated queries ship in parallel so existing local views keep
// working.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
)

// FederationProvider exposes the hub-side state required by the federated
// query resolvers. The Resolver is wired with a nil provider on non-hub
// servers; resolvers degrade to empty-list responses in that case.
type FederationProvider interface {
	// Spokes returns a snapshot of registered spokes. Implementations should
	// return a copy that callers may sort or filter without affecting the
	// underlying registry.
	Spokes() []federation.SpokeRecord
	// FanOut executes the supplied request against every spoke returned by
	// Spokes (the implementation may apply its own active/stale policy) and
	// returns the merged FanOutResult.
	FanOut(ctx context.Context, req *federation.FanOutRequest) (*federation.FanOutResult, error)
}

// federationCapWrite is the capability string a spoke advertises when it
// allows writes from federated callers. Read-only fan-out is the v1 default
// so this field is always false until a spoke explicitly opts in.
const federationCapWrite = "write"

// federationIdentityPrefix mirrors the synthetic capability used by the hub
// to stash the identity fingerprint on a SpokeRecord. Filtered out of the
// FederationNode.capabilities surface so clients never see it.
const federationIdentityPrefix = "@identity:"

// FederationNodes returns one row per registered spoke decorated with the
// most recent fan-out outcome. When the server is not in hub mode (Resolver
// has no FederationProvider) the result is an empty list.
func (r *queryResolver) FederationNodes(ctx context.Context) ([]*FederationNode, error) {
	if r.Federation == nil {
		return []*FederationNode{}, nil
	}
	spokes := r.Federation.Spokes()
	out := make([]*FederationNode, 0, len(spokes))
	for _, s := range spokes {
		out = append(out, federationNodeFromSpoke(s, ""))
	}
	return out, nil
}

// FederatedBeads fans out a beads query and merges the resulting edges into
// FederatedBead rows.
func (r *queryResolver) FederatedBeads(ctx context.Context, status *string, label *string, projectID *string) ([]*FederatedBead, error) {
	if r.Federation == nil {
		return []*FederatedBead{}, nil
	}
	const beadsQuery = `query FedBeads($status:String,$label:String,$projectID:String){
  beads(first:200,status:$status,label:$label,projectID:$projectID){
    edges{node{id title status priority issueType owner createdAt createdBy updatedAt labels projectID parent description acceptance notes}}
  }
}`
	vars := map[string]any{}
	if status != nil {
		vars["status"] = *status
	}
	if label != nil {
		vars["label"] = *label
	}
	if projectID != nil {
		vars["projectID"] = *projectID
	}
	res, err := r.Federation.FanOut(ctx, &federation.FanOutRequest{Query: beadsQuery, Variables: vars})
	if err != nil {
		return nil, err
	}
	spokeIndex := indexSpokes(r.Federation.Spokes())
	out := []*FederatedBead{}
	for _, n := range res.Nodes {
		if n.Outcome != federation.OutcomeOK {
			continue
		}
		var resp struct {
			Data struct {
				Beads struct {
					Edges []struct {
						Node *Bead `json:"node"`
					} `json:"edges"`
				} `json:"beads"`
			} `json:"data"`
		}
		if err := json.Unmarshal(n.Response, &resp); err != nil {
			continue
		}
		s := spokeIndex[n.NodeID]
		writeCap := hasCapability(s.Capabilities, federationCapWrite)
		statusStr := string(s.Status)
		for _, e := range resp.Data.Beads.Edges {
			if e.Node == nil {
				continue
			}
			out = append(out, &FederatedBead{
				NodeID:          n.NodeID,
				ProjectID:       e.Node.ProjectID,
				ProjectURL:      n.URL,
				WriteCapability: writeCap,
				Status:          statusStr,
				Bead:            e.Node,
			})
		}
	}
	return out, nil
}

// FederatedRuns fans out a runs query and merges the resulting edges into
// FederatedRun rows.
func (r *queryResolver) FederatedRuns(ctx context.Context, layer *RunLayer, projectID *string) ([]*FederatedRun, error) {
	if r.Federation == nil {
		return []*FederatedRun{}, nil
	}
	const runsQuery = `query FedRuns($layer:RunLayer,$projectID:String){
  runs(first:200,layer:$layer,projectID:$projectID){
    edges{node{id layer status projectID startedAt completedAt beadId artifactId parentRunId childRunIds}}
  }
}`
	vars := map[string]any{}
	if layer != nil {
		vars["layer"] = string(*layer)
	}
	if projectID != nil {
		vars["projectID"] = *projectID
	}
	res, err := r.Federation.FanOut(ctx, &federation.FanOutRequest{Query: runsQuery, Variables: vars})
	if err != nil {
		return nil, err
	}
	spokeIndex := indexSpokes(r.Federation.Spokes())
	out := []*FederatedRun{}
	for _, n := range res.Nodes {
		if n.Outcome != federation.OutcomeOK {
			continue
		}
		var resp struct {
			Data struct {
				Runs struct {
					Edges []struct {
						Node *Run `json:"node"`
					} `json:"edges"`
				} `json:"runs"`
			} `json:"data"`
		}
		if err := json.Unmarshal(n.Response, &resp); err != nil {
			continue
		}
		s := spokeIndex[n.NodeID]
		writeCap := hasCapability(s.Capabilities, federationCapWrite)
		statusStr := string(s.Status)
		for _, e := range resp.Data.Runs.Edges {
			if e.Node == nil {
				continue
			}
			out = append(out, &FederatedRun{
				NodeID:          n.NodeID,
				ProjectID:       e.Node.ProjectID,
				ProjectURL:      n.URL,
				WriteCapability: writeCap,
				Status:          statusStr,
				Run:             e.Node,
			})
		}
	}
	return out, nil
}

// FederatedProjects fans out a projects query and merges the resulting edges
// into FederatedProject rows.
func (r *queryResolver) FederatedProjects(ctx context.Context, includeUnreachable *bool) ([]*FederatedProject, error) {
	if r.Federation == nil {
		return []*FederatedProject{}, nil
	}
	const projectsQuery = `query FedProjects($inc:Boolean){
  projects(first:200,includeUnreachable:$inc){
    edges{node{id name path gitRemote registeredAt lastSeen unreachable tombstonedAt}}
  }
}`
	vars := map[string]any{}
	if includeUnreachable != nil {
		vars["inc"] = *includeUnreachable
	}
	res, err := r.Federation.FanOut(ctx, &federation.FanOutRequest{Query: projectsQuery, Variables: vars})
	if err != nil {
		return nil, err
	}
	spokeIndex := indexSpokes(r.Federation.Spokes())
	out := []*FederatedProject{}
	for _, n := range res.Nodes {
		if n.Outcome != federation.OutcomeOK {
			continue
		}
		var resp struct {
			Data struct {
				Projects struct {
					Edges []struct {
						Node *Project `json:"node"`
					} `json:"edges"`
				} `json:"projects"`
			} `json:"data"`
		}
		if err := json.Unmarshal(n.Response, &resp); err != nil {
			continue
		}
		s := spokeIndex[n.NodeID]
		writeCap := hasCapability(s.Capabilities, federationCapWrite)
		statusStr := string(s.Status)
		for _, e := range resp.Data.Projects.Edges {
			if e.Node == nil {
				continue
			}
			out = append(out, &FederatedProject{
				NodeID:          n.NodeID,
				ProjectID:       e.Node.ID,
				ProjectURL:      n.URL,
				WriteCapability: writeCap,
				Status:          statusStr,
				Project:         e.Node,
			})
		}
	}
	return out, nil
}

// federationNodeFromSpoke materializes one FederationNode row, filtering the
// synthetic identity capability and stamping a lastError diagnostic when
// supplied (e.g. for a version-skew handshake).
func federationNodeFromSpoke(s federation.SpokeRecord, lastError string) *FederationNode {
	caps := make([]string, 0, len(s.Capabilities))
	for _, c := range s.Capabilities {
		if strings.HasPrefix(c, federationIdentityPrefix) {
			continue
		}
		caps = append(caps, c)
	}
	n := &FederationNode{
		ID:              s.NodeID,
		NodeID:          s.NodeID,
		Name:            s.Name,
		URL:             s.URL,
		Status:          string(s.Status),
		DdxVersion:      s.DDxVersion,
		SchemaVersion:   s.SchemaVersion,
		Capabilities:    caps,
		RegisteredAt:    s.RegisteredAt.UTC().Format(time.RFC3339),
		WriteCapability: hasCapability(caps, federationCapWrite),
	}
	if !s.LastHeartbeat.IsZero() {
		ts := s.LastHeartbeat.UTC().Format(time.RFC3339)
		n.LastHeartbeat = &ts
	}
	if lastError != "" {
		le := lastError
		n.LastError = &le
	}
	return n
}

func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func indexSpokes(spokes []federation.SpokeRecord) map[string]federation.SpokeRecord {
	out := make(map[string]federation.SpokeRecord, len(spokes))
	for _, s := range spokes {
		out[s.NodeID] = s
	}
	return out
}
