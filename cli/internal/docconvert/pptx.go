package docconvert

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// OOXML namespaces used by a .pptx. DrawingML carries the text, tables and
// line breaks; PresentationML carries the non-visual shape properties that
// hold image alt text.
const (
	drawingMLNS      = "http://schemas.openxmlformats.org/drawingml/2006/main"
	presentationMLNS = "http://schemas.openxmlformats.org/presentationml/2006/main"
)

var (
	slidePartPattern = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	notesPartPattern = regexp.MustCompile(`^ppt/notesSlides/notesSlide(\d+)\.xml$`)
)

// pptxConverter renders a PowerPoint deck to Markdown.
//
// Ported from the reference implementation in
// synaptiq-skadden-hq/tools/pptx_to_md.py, which this converter is expected to
// agree with: slides in archive-index order, one bullet per non-empty
// paragraph, tables rendered as pipe rows, image alt text from the shape
// `descr` attribute, and speaker notes quoted underneath the slide.
type pptxConverter struct{}

func (pptxConverter) Extensions() []string { return []string{".pptx"} }

func (c pptxConverter) Convert(absPath, sourceLabel string) (string, error) {
	reader, err := zip.OpenReader(absPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", sourceLabel, err)
	}
	defer func() { _ = reader.Close() }()

	slides := orderedParts(reader.File, slidePartPattern)
	if len(slides) == 0 {
		return "", fmt.Errorf("read %s: no slides found; is this a .pptx?", sourceLabel)
	}

	// Notes are keyed by the notesSlide part index and looked up below by the
	// slide part index. That is the reference implementation's behaviour and
	// holds for decks PowerPoint numbers in step. It is not the authoritative
	// mapping: notesSlideN.xml is bound to its slide through
	// ppt/slides/_rels/slideN.xml.rels, and a deck whose notes were authored
	// out of order can pair them differently. Reading the rels is the fix when
	// a deck shows up mismatched; changing it now would diverge from the
	// reference for no observed case.
	notes := map[int]string{}
	for _, part := range orderedParts(reader.File, notesPartPattern) {
		root, err := readPart(part.file)
		if err != nil {
			return "", fmt.Errorf("read %s (%s): %w", sourceLabel, part.name, err)
		}
		text := strings.Join(shapeParagraphs(root), "\n")
		if strings.TrimSpace(text) != "" {
			notes[part.index] = text
		}
	}

	stem := strings.TrimSuffix(filepath.Base(sourceLabel), filepath.Ext(sourceLabel))
	lines := []string{
		"# " + stem,
		"",
		"> **Generated file — do not edit.** Text extraction of the artifact of " +
			"record, which is authored in an external tool. Regenerate with " +
			"`ddx artifact check-in`.",
		"",
		fmt.Sprintf("- **Source**: `%s`", sourceLabel),
		fmt.Sprintf("- **Slides**: %d", len(slides)),
		"",
		"---",
		"",
	}

	for _, part := range slides {
		root, err := readPart(part.file)
		if err != nil {
			return "", fmt.Errorf("read %s (%s): %w", sourceLabel, part.name, err)
		}

		lines = append(lines, fmt.Sprintf("## Slide %d", part.index), "")

		paragraphs := shapeParagraphs(root)
		if len(paragraphs) > 0 {
			for _, text := range paragraphs {
				for _, piece := range strings.Split(text, "\n") {
					piece = strings.TrimSpace(piece)
					if piece != "" {
						lines = append(lines, "- "+piece)
					}
				}
			}
			lines = append(lines, "")
		}

		for _, rows := range shapeTables(root) {
			for _, row := range rows {
				cells := make([]string, 0, len(row))
				for _, cell := range row {
					cells = append(cells, strings.ReplaceAll(cell, "|", `\|`))
				}
				lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
			}
			lines = append(lines, "")
		}

		if alts := shapeAltTexts(root); len(alts) > 0 {
			lines = append(lines, "**Image descriptions**", "")
			for _, alt := range alts {
				lines = append(lines, "- "+alt)
			}
			lines = append(lines, "")
		}

		if note, ok := notes[part.index]; ok {
			lines = append(lines, "**Speaker notes**", "")
			for _, piece := range strings.Split(note, "\n") {
				piece = strings.TrimSpace(piece)
				if piece != "" {
					lines = append(lines, "> "+piece)
				}
			}
			lines = append(lines, "")
		}
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// zipPart is one numbered part of the archive.
type zipPart struct {
	index int
	name  string
	file  *zip.File
}

// orderedParts selects the archive entries matching pattern and sorts them
// numerically by their captured index. Archive order is not guaranteed to be
// numeric (slide10 sorts before slide2 lexically, and writers are free to
// store parts in any order), so sorting here is what makes the rendering
// deterministic.
func orderedParts(files []*zip.File, pattern *regexp.Regexp) []zipPart {
	var parts []zipPart
	for _, file := range files {
		match := pattern.FindStringSubmatch(file.Name)
		if match == nil {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		parts = append(parts, zipPart{index: index, name: file.Name, file: file})
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].index != parts[j].index {
			return parts[i].index < parts[j].index
		}
		return parts[i].name < parts[j].name
	})
	return parts
}

func readPart(file *zip.File) (*xmlNode, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return parseXML(data)
}

// paragraphText joins a paragraph's runs.
//
// PowerPoint splits a single word across several <a:t> runs whenever
// formatting, spell-check state or revision metadata changes mid-word.
// Concatenating the run texts with no separator is what puts the word back
// together; inserting spaces or newlines between runs corrupts the text. Only
// an explicit <a:br> is a line break.
func paragraphText(para *xmlNode) string {
	var b strings.Builder
	para.walk(func(node *xmlNode) {
		switch {
		case node.is(drawingMLNS, "t"):
			b.WriteString(node.Text)
		case node.is(drawingMLNS, "br"):
			b.WriteString("\n")
		}
	})
	return strings.TrimSpace(b.String())
}

// shapeParagraphs returns the non-empty paragraphs anywhere under root, in
// document order. Table cell paragraphs are included: the deck's text is
// rendered as bullets for readability and grep, and the table is rendered
// again below as a grid.
func shapeParagraphs(root *xmlNode) []string {
	var out []string
	for _, para := range root.descendants(drawingMLNS, "p") {
		if text := paragraphText(para); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// shapeTables returns every table under root as rows of cell strings.
func shapeTables(root *xmlNode) [][][]string {
	var tables [][][]string
	for _, tbl := range root.descendants(drawingMLNS, "tbl") {
		var rows [][]string
		for _, tr := range tbl.childElements(drawingMLNS, "tr") {
			var cells []string
			for _, tc := range tr.childElements(drawingMLNS, "tc") {
				cells = append(cells, strings.Join(shapeParagraphs(tc), " "))
			}
			rows = append(rows, cells)
		}
		if len(rows) > 0 {
			tables = append(tables, rows)
		}
	}
	return tables
}

// shapeAltTexts returns the alt text authors attached to pictures and shapes,
// carried on the descr attribute of the non-visual drawing properties.
func shapeAltTexts(root *xmlNode) []string {
	var out []string
	for _, pr := range root.descendants(presentationMLNS, "cNvPr") {
		if descr := strings.TrimSpace(pr.attr("descr")); descr != "" {
			out = append(out, descr)
		}
	}
	return out
}
