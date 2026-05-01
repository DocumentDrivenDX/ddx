package graphql

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// beadRefPattern matches bead IDs of the form ddx-<8 hex chars>.
var beadRefPattern = regexp.MustCompile(`ddx-[a-f0-9]{8}`)

// Commits is the resolver for the commits field.
func (r *queryResolver) Commits(ctx context.Context, projectID string, first *int, after *string, last *int, before *string, since *string, author *string) (*CommitConnection, error) {

	proj, ok := r.State.GetProjectSnapshotByID(projectID)
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	snaps, err := gitLogCommits(proj.Path, since, author)
	if err != nil {
		return nil, err
	}

	// Build full edge list with stable SHA-based cursors.
	all := make([]*CommitEdge, len(snaps))
	for i, c := range snaps {
		all[i] = &CommitEdge{
			Node:   c,
			Cursor: encodeStableCursor(c.Sha),
		}
	}

	// Apply window: start after `after` cursor, end before `before` cursor.
	startIdx := 0
	if after != nil {
		if afterSha, ok := decodeStableCursor(*after); ok {
			for i, e := range all {
				if e.Node.Sha == afterSha {
					startIdx = i + 1
					break
				}
			}
		}
	}
	endIdx := len(all)
	if before != nil {
		if beforeSha, ok := decodeStableCursor(*before); ok {
			for i, e := range all {
				if e.Node.Sha == beforeSha {
					endIdx = i
					break
				}
			}
		}
	}
	if startIdx > endIdx {
		startIdx = endIdx
	}

	windowSize := endIdx - startIdx
	slice := all[startIdx:endIdx]
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
	}
	if last != nil && *last >= 0 && *last < len(slice) {
		slice = slice[len(slice)-*last:]
	}

	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0 || (last != nil && *last >= 0 && *last < windowSize),
		HasNextPage:     endIdx < len(all) || (first != nil && *first >= 0 && *first < windowSize),
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}

	return &CommitConnection{
		Edges:      slice,
		PageInfo:   pageInfo,
		TotalCount: len(all),
	}, nil
}

// gitLogCommits runs git log for the given project path and returns parsed commits.
// Up to 500 commits are fetched. Optionally filtered by since (ISO-8601) and author.
func gitLogCommits(path string, since, author *string) ([]*Commit, error) {
	const sep = "\x1f"
	const recSep = "\x1e"
	format := "--pretty=format:%H" + sep + "%h" + sep + "%an" + sep + "%aI" + sep + "%s" + sep + "%b" + recSep

	args := []string{"-C", path, "log", format, "-n", strconv.Itoa(500)}
	if since != nil && *since != "" {
		args = append(args, "--since="+*since)
	}
	if author != nil && *author != "" {
		args = append(args, "--author="+*author)
	}

	cmd := exec.Command("git", args...) //nolint:gosec
	cmd.Env = graphqlScrubbedGitEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	var commits []*Commit
	records := strings.Split(string(out), recSep)
	for _, rec := range records {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		fields := strings.SplitN(rec, sep, 6)
		if len(fields) < 6 {
			continue
		}
		body := fields[5]
		refs := beadRefPattern.FindAllString(fields[4]+"\n"+body, -1)
		if refs == nil {
			refs = []string{}
		}
		c := &Commit{
			Sha:      fields[0],
			ShortSha: fields[1],
			Author:   fields[2],
			Date:     fields[3],
			Subject:  fields[4],
			BeadRefs: refs,
		}
		if body != "" {
			c.Body = &body
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// graphqlScrubbedGitEnv returns the current process environment with GIT_*
// variables removed so git subcommands honour explicit -C flags.
func graphqlScrubbedGitEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
