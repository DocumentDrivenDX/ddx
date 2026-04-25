package evidence

import "strings"

// DiffFile is one file's slice of a unified diff. Stat is the
// "+N -N" or "Bin" summary line; HunkHeaders are the "@@ ..." lines
// in original order; Body is the full per-file diff text including
// the "diff --git" header.
type DiffFile struct {
	Path        string
	Stat        string
	HunkHeaders []string
	Body        string
}

// DecomposeDiff parses a unified diff into per-file slices. It supports
// the standard "diff --git a/<path> b/<path>" delimiter produced by
// git. Files that lack a parseable header are returned with an empty
// Path. The returned slice preserves input order.
//
// FEAT-022 §1: diff decomposer returns file inventory, per-file stat
// lines, and hunk headers so large diffs can degrade to
// stat + hunk-headers only.
func DecomposeDiff(diff string) []DiffFile {
	if diff == "" {
		return nil
	}
	lines := strings.Split(diff, "\n")
	var files []DiffFile
	var cur *DiffFile

	flush := func() {
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			path := parseDiffGitPath(line)
			cur = &DiffFile{Path: path}
		}
		if cur == nil {
			continue
		}
		// Append line to body (preserve trailing newlines except final empty).
		if i < len(lines)-1 {
			cur.Body += line + "\n"
		} else {
			cur.Body += line
		}
		switch {
		case strings.HasPrefix(line, "@@ "):
			cur.HunkHeaders = append(cur.HunkHeaders, line)
		case strings.HasPrefix(line, "Binary files "):
			cur.Stat = "Bin"
		}
	}
	flush()
	return files
}

func parseDiffGitPath(header string) string {
	// "diff --git a/<path> b/<path>"
	rest := strings.TrimPrefix(header, "diff --git ")
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) == 0 {
		return ""
	}
	a := parts[0]
	a = strings.TrimPrefix(a, "a/")
	return a
}

// ClampDiff bounds a unified diff to max bytes. When the diff fits, it
// returns the diff unchanged. When it does not, it degrades each file
// to "stat + hunk headers only" in input order until the budget is
// reached, then truncates the remainder. Files that don't fit at all
// are listed as omitted.
//
// FEAT-022 §1: large diffs degrade to stat + hunk-headers only.
func ClampDiff(diff string, max int) (clamped string, section EvidenceAssemblySection) {
	section = EvidenceAssemblySection{Name: "diff"}
	if len(diff) <= max {
		section.BytesIncluded = len(diff)
		return diff, section
	}
	files := DecomposeDiff(diff)
	if len(files) == 0 {
		out, truncated, orig := ClampOutput(diff, max)
		section.BytesIncluded = len(out)
		section.BytesOmitted = orig - len(out)
		if truncated {
			section.TruncationReason = "byte_cap"
		}
		return out, section
	}

	var b strings.Builder
	used := 0
	reserve := len(TruncationMarker)
	for _, f := range files {
		// Try full body first.
		if used+len(f.Body) <= max {
			b.WriteString(f.Body)
			used += len(f.Body)
			section.SelectedItems = append(section.SelectedItems, f.Path)
			continue
		}
		// Degrade: stat + hunk headers only.
		degraded := degradedFile(f)
		if used+len(degraded)+reserve <= max {
			b.WriteString(degraded)
			used += len(degraded)
			section.SelectedItems = append(section.SelectedItems, f.Path)
			section.BytesOmitted += len(f.Body) - len(degraded)
			if section.TruncationReason == "" {
				section.TruncationReason = "diff_degraded"
			}
			continue
		}
		// No room even for degraded form: omit.
		section.OmittedItems = append(section.OmittedItems, f.Path)
		section.BytesOmitted += len(f.Body)
		if section.TruncationReason == "" {
			section.TruncationReason = "byte_cap"
		}
	}
	if section.BytesOmitted > 0 {
		b.WriteString(TruncationMarker)
	}
	out := b.String()
	section.BytesIncluded = len(out)
	return out, section
}

func degradedFile(f DiffFile) string {
	var b strings.Builder
	b.WriteString("diff --git a/")
	b.WriteString(f.Path)
	b.WriteString(" b/")
	b.WriteString(f.Path)
	b.WriteString("\n")
	if f.Stat != "" {
		b.WriteString(f.Stat)
		b.WriteString("\n")
	}
	for _, h := range f.HunkHeaders {
		b.WriteString(h)
		b.WriteString("\n")
	}
	return b.String()
}
