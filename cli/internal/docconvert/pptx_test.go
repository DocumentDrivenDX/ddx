package docconvert

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slideXML wraps shape bodies in the minimal slide part PowerPoint writes.
func slideXML(shapes string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld>
</p:sld>`
}

func notesXML(paragraphs string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld><p:spTree><p:sp><p:txBody>` + paragraphs + `</p:txBody></p:sp></p:spTree></p:cSld>
</p:notes>`
}

// writePPTX builds a .pptx fixture from the given part name -> XML map. Parts
// are written in the order given so tests can scramble archive order.
func writePPTX(t *testing.T, path string, parts [][2]string) {
	t.Helper()

	file, err := os.Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	archive := zip.NewWriter(file)
	for _, part := range parts {
		writer, err := archive.Create(part[0])
		require.NoError(t, err)
		_, err = writer.Write([]byte(part[1]))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
}

// fixtureParts is the canonical fixture deck: three slides whose archive
// indices are 1, 2 and 10 (so lexical ordering would put 10 second), a title
// split across runs mid-word, an explicit line break, a table, image alt text,
// and speaker notes.
func fixtureParts() [][2]string {
	return [][2]string{
		{"ppt/slides/slide1.xml", slideXML(
			`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/></p:nvSpPr><p:txBody>` +
				`<a:p><a:r><a:t>Prebi</a:t></a:r><a:r><a:t>ll rev</a:t></a:r><a:r><a:t>iew</a:t></a:r></a:p>` +
				`<a:p><a:r><a:t>First line</a:t></a:r><a:br/><a:r><a:t>Second line</a:t></a:r></a:p>` +
				`</p:txBody></p:sp>`)},
		{"ppt/slides/slide10.xml", slideXML(
			`<p:sp><p:nvSpPr><p:cNvPr id="4" name="Body"/></p:nvSpPr><p:txBody>` +
				`<a:p><a:r><a:t>Last slide</a:t></a:r></a:p></p:txBody></p:sp>` +
				`<p:pic><p:nvPicPr><p:cNvPr id="5" name="Chart" descr="Fee curve by month"/></p:nvPicPr></p:pic>`)},
		{"ppt/slides/slide2.xml", slideXML(
			`<p:graphicFrame><a:graphic><a:graphicData><a:tbl>` +
				`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>Tier</a:t></a:r></a:p></a:txBody></a:tc>` +
				`<a:tc><a:txBody><a:p><a:r><a:t>Rate | cap</a:t></a:r></a:p></a:txBody></a:tc></a:tr>` +
				`<a:tr><a:tc><a:txBody><a:p><a:r><a:t>Partner</a:t></a:r></a:p></a:txBody></a:tc>` +
				`<a:tc><a:txBody><a:p><a:r><a:t>1200</a:t></a:r></a:p></a:txBody></a:tc></a:tr>` +
				`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`)},
		{"ppt/notesSlides/notesSlide1.xml", notesXML(
			`<a:p><a:r><a:t>Open with the fee-application numbers.</a:t></a:r></a:p>`)},
	}
}

func fixtureDeck(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SD-004-prebill-review.pptx")
	writePPTX(t, path, fixtureParts())
	return path
}

func TestPPTXConvertRendersDeck(t *testing.T) {
	out, err := Convert(fixtureDeck(t), "docs/assets/SD-004-prebill-review.pptx")
	require.NoError(t, err)

	expected := "# SD-004-prebill-review\n" +
		"\n" +
		"> **Generated file — do not edit.** Text extraction of the artifact of record, " +
		"which is authored in an external tool. Regenerate with `ddx artifact check-in`.\n" +
		"\n" +
		"- **Source**: `docs/assets/SD-004-prebill-review.pptx`\n" +
		"- **Slides**: 3\n" +
		"\n" +
		"---\n" +
		"\n" +
		"## Slide 1\n" +
		"\n" +
		"- Prebill review\n" +
		"- First line\n" +
		"- Second line\n" +
		"\n" +
		"**Speaker notes**\n" +
		"\n" +
		"> Open with the fee-application numbers.\n" +
		"\n" +
		"## Slide 2\n" +
		"\n" +
		"- Tier\n" +
		"- Rate | cap\n" +
		"- Partner\n" +
		"- 1200\n" +
		"\n" +
		"| Tier | Rate \\| cap |\n" +
		"| Partner | 1200 |\n" +
		"\n" +
		"## Slide 10\n" +
		"\n" +
		"- Last slide\n" +
		"\n" +
		"**Image descriptions**\n" +
		"\n" +
		"- Fee curve by month\n"

	assert.Equal(t, expected, out)
}

// PowerPoint splits a single word across runs whenever formatting or
// spell-check metadata changes mid-word. Joining runs with anything but the
// empty string corrupts the text, so this is asserted on its own.
func TestPPTXConvertJoinsRunsWithoutSeparator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.pptx")
	writePPTX(t, path, [][2]string{{"ppt/slides/slide1.xml", slideXML(
		`<p:sp><p:txBody><a:p>` +
			`<a:r><a:t>reconcil</a:t></a:r><a:r><a:t>iation </a:t></a:r><a:r><a:t>pass</a:t></a:r>` +
			`</a:p></p:txBody></p:sp>`)}})

	out, err := Convert(path, "runs.pptx")
	require.NoError(t, err)
	assert.Contains(t, out, "- reconciliation pass\n")
	assert.NotContains(t, out, "reconcil iation")
}

// Determinism is the whole point of committing the rendering: a non-stable
// ordering turns every check-in into a meaningless diff. Repeated conversions
// of the same bytes, and conversions of archives whose parts were stored in a
// different order, must all agree.
func TestPPTXConvertIsDeterministic(t *testing.T) {
	dir := t.TempDir()

	forward := filepath.Join(dir, "forward.pptx")
	writePPTX(t, forward, fixtureParts())

	reversed := fixtureParts()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	shuffled := filepath.Join(dir, "shuffled.pptx")
	writePPTX(t, shuffled, reversed)

	first, err := Convert(forward, "deck.pptx")
	require.NoError(t, err)

	for i := 0; i < 25; i++ {
		again, err := Convert(forward, "deck.pptx")
		require.NoError(t, err)
		require.Equal(t, first, again, "conversion %d differed from the first", i)
	}

	fromShuffled, err := Convert(shuffled, "deck.pptx")
	require.NoError(t, err)
	assert.Equal(t, first, fromShuffled, "archive storage order changed the rendering")

	// Slides must come out in numeric index order, not lexical order.
	assert.Less(t, strings.Index(first, "## Slide 2"), strings.Index(first, "## Slide 10"))
}

func TestPPTXConvertRejectsArchiveWithoutSlides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.pptx")
	writePPTX(t, path, [][2]string{{"docProps/app.xml", "<Properties/>"}})

	_, err := Convert(path, "empty.pptx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no slides found")
}

func TestConvertRejectsUnsupportedExtension(t *testing.T) {
	_, err := For("docs/design/deck.key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no converter for .key files")
	assert.Contains(t, err.Error(), ".pptx")

	_, err = For("README")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file extension")
}

func TestForIsCaseInsensitive(t *testing.T) {
	converter, err := For("DECK.PPTX")
	require.NoError(t, err)
	assert.Equal(t, []string{".pptx"}, converter.Extensions())
}

func TestSupportedExtensionsIsSorted(t *testing.T) {
	assert.Equal(t, []string{".pptx"}, SupportedExtensions())
}
