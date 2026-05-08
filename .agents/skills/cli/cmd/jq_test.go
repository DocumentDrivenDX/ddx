package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runJqCommand runs "ddx jq <args...>" with the given stdin content (empty = no stdin override).
// When stdinContent is non-empty it is written to a temp file and passed as an explicit file arg,
// because the implementation reads os.Stdin directly and we cannot swap it in tests.
func runJqCommand(t *testing.T, stdinContent string, args ...string) (string, error) {
	t.Helper()
	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()

	finalArgs := append([]string{"jq"}, args...)

	// If the caller wants to simulate stdin input, write it to a temp file
	// and append the file path — unless -n or --null-input is in the args.
	if stdinContent != "" {
		nullInput := false
		for _, a := range args {
			if a == "-n" || a == "--null-input" {
				nullInput = true
				break
			}
		}
		if !nullInput {
			tmp := filepath.Join(t.TempDir(), "input.json")
			require.NoError(t, os.WriteFile(tmp, []byte(stdinContent), 0644))
			finalArgs = append(finalArgs, tmp)
		}
	}

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(finalArgs)

	err := root.Execute()
	return buf.String(), err
}

// --- parseJqArgs unit tests ---

func TestParseJqArgs_BasicFilter(t *testing.T) {
	opts, err := parseJqArgs([]string{".foo"})
	require.NoError(t, err)
	assert.Equal(t, ".foo", opts.filter)
	assert.Empty(t, opts.files)
}

func TestParseJqArgs_FilterThenFiles(t *testing.T) {
	opts, err := parseJqArgs([]string{".foo", "a.json", "b.json"})
	require.NoError(t, err)
	assert.Equal(t, ".foo", opts.filter)
	assert.Equal(t, []string{"a.json", "b.json"}, opts.files)
}

func TestParseJqArgs_ShortFlags(t *testing.T) {
	opts, err := parseJqArgs([]string{"-r", "-c", "-s", "-n", "-e", "-S", ".x"})
	require.NoError(t, err)
	assert.True(t, opts.rawOutput)
	assert.True(t, opts.compact)
	assert.True(t, opts.slurp)
	assert.True(t, opts.nullInput)
	assert.True(t, opts.exitStatus)
	assert.True(t, opts.sortKeys)
}

func TestParseJqArgs_CombinedShortFlags(t *testing.T) {
	opts, err := parseJqArgs([]string{"-rc", ".x"})
	require.NoError(t, err)
	assert.True(t, opts.rawOutput)
	assert.True(t, opts.compact)
}

func TestParseJqArgs_LongFlags(t *testing.T) {
	opts, err := parseJqArgs([]string{"--raw-output", "--compact-output", "--slurp", "--null-input", "."})
	require.NoError(t, err)
	assert.True(t, opts.rawOutput)
	assert.True(t, opts.compact)
	assert.True(t, opts.slurp)
	assert.True(t, opts.nullInput)
}

func TestParseJqArgs_ArgFlag(t *testing.T) {
	opts, err := parseJqArgs([]string{"--arg", "name", "alice", "."})
	require.NoError(t, err)
	assert.Equal(t, "alice", opts.variables["name"])
}

func TestParseJqArgs_ArgJsonFlag(t *testing.T) {
	opts, err := parseJqArgs([]string{"--argjson", "count", "42", "."})
	require.NoError(t, err)
	assert.EqualValues(t, float64(42), opts.variables["count"])
}

func TestParseJqArgs_ArgJsonInvalid(t *testing.T) {
	_, err := parseJqArgs([]string{"--argjson", "x", "not-json", "."})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--argjson")
}

func TestParseJqArgs_IndentFlag(t *testing.T) {
	opts, err := parseJqArgs([]string{"--indent", "4", "."})
	require.NoError(t, err)
	assert.Equal(t, 4, opts.indent)
}

func TestParseJqArgs_IndentMissingValue(t *testing.T) {
	_, err := parseJqArgs([]string{"--indent"})
	require.Error(t, err)
}

func TestParseJqArgs_TabFlag(t *testing.T) {
	opts, err := parseJqArgs([]string{"--tab", "."})
	require.NoError(t, err)
	assert.True(t, opts.tab)
}

func TestParseJqArgs_JoinOutput(t *testing.T) {
	opts, err := parseJqArgs([]string{"-j", "."})
	require.NoError(t, err)
	assert.True(t, opts.joinOutput)
	assert.True(t, opts.rawOutput) // -j implies -r
}

func TestParseJqArgs_HelpFlag(t *testing.T) {
	opts, err := parseJqArgs([]string{"--help"})
	require.NoError(t, err)
	assert.True(t, opts.help)
}

func TestParseJqArgs_VersionFlag(t *testing.T) {
	opts, err := parseJqArgs([]string{"-V"})
	require.NoError(t, err)
	assert.True(t, opts.version)
}

func TestParseJqArgs_DashDash(t *testing.T) {
	opts, err := parseJqArgs([]string{"--", ".foo", "file.json"})
	require.NoError(t, err)
	assert.Equal(t, ".foo", opts.filter)
	assert.Equal(t, []string{"file.json"}, opts.files)
}

func TestParseJqArgs_UnknownFlag(t *testing.T) {
	_, err := parseJqArgs([]string{"-z", "."})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-z")
}

// --- Full command integration tests (using temp files for input) ---

func TestJqCommand_BasicFilter(t *testing.T) {
	out, err := runJqCommand(t, `{"a":1}`, ".a")
	require.NoError(t, err)
	assert.Equal(t, "1\n", out)
}

func TestJqCommand_RawOutput(t *testing.T) {
	out, err := runJqCommand(t, `{"name":"alice"}`, "-r", ".name")
	require.NoError(t, err)
	assert.Equal(t, "alice\n", out)
}

func TestJqCommand_CompactOutput(t *testing.T) {
	out, err := runJqCommand(t, `{"a":1,"b":2}`, "-c", ".")
	require.NoError(t, err)
	assert.Equal(t, `{"a":1,"b":2}`+"\n", out)
}

func TestJqCommand_NullInput(t *testing.T) {
	out, err := runJqCommand(t, "", "-n", "[1,2,3]")
	require.NoError(t, err)
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "2")
	assert.Contains(t, out, "3")
}

func TestJqCommand_NullInputRange(t *testing.T) {
	out, err := runJqCommand(t, "", "-n", "-c", "[range(3)]")
	require.NoError(t, err)
	assert.Equal(t, "[0,1,2]\n", out)
}

func TestJqCommand_Slurp(t *testing.T) {
	// Multiple JSON values slurped into array
	out, err := runJqCommand(t, "1\n2\n3\n", "-s", "-c", ".")
	require.NoError(t, err)
	assert.Equal(t, "[1,2,3]\n", out)
}

func TestJqCommand_ArgVariable(t *testing.T) {
	out, err := runJqCommand(t, `{}`, "--arg", "key", "hello", ".key = $key")
	require.NoError(t, err)
	assert.Contains(t, out, `"hello"`)
}

func TestJqCommand_ArgJsonVariable(t *testing.T) {
	out, err := runJqCommand(t, `{}`, "--argjson", "n", "42", ".n = $n")
	require.NoError(t, err)
	assert.Contains(t, out, "42")
}

func TestJqCommand_MultipleInputValues(t *testing.T) {
	// JSONL: multiple objects, extract a field from each
	out, err := runJqCommand(t, "{\"x\":1}\n{\"x\":2}\n", "-r", ".x | tostring")
	require.NoError(t, err)
	assert.Equal(t, "1\n2\n", out)
}

func TestJqCommand_InvalidFilter(t *testing.T) {
	out, err := runJqCommand(t, `{}`, "not valid filter !!!")
	// Should return an error
	require.Error(t, err)
	_ = out
}

func TestJqCommand_NoFilter(t *testing.T) {
	_, err := runJqCommand(t, "")
	require.Error(t, err)
}

func TestJqCommand_Help(t *testing.T) {
	out, err := runJqCommand(t, "", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "FILTER")
	assert.Contains(t, out, "--raw-output")
}

func TestJqCommand_Version(t *testing.T) {
	out, err := runJqCommand(t, "", "--version")
	require.NoError(t, err)
	assert.Contains(t, out, "gojq")
}

func TestJqCommand_FileInput(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(tmp, []byte(`{"z":99}`), 0644))

	factory := NewCommandFactory(t.TempDir())
	root := factory.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"jq", ".z", tmp})

	require.NoError(t, root.Execute())
	assert.Equal(t, "99\n", buf.String())
}

func TestJqCommand_SlurpFile(t *testing.T) {
	sf := filepath.Join(t.TempDir(), "lookup.json")
	require.NoError(t, os.WriteFile(sf, []byte(`{"greeting":"hi"}`), 0644))

	out, err := runJqCommand(t, `"world"`, "--slurpfile", "lup", sf, "-r", "$lup[0].greeting")
	require.NoError(t, err)
	assert.Equal(t, "hi", strings.TrimSpace(out))
}

func TestJqCommand_CombinedFlagsRC(t *testing.T) {
	// -rc = raw-output + compact: string values printed without quotes
	out, err := runJqCommand(t, `{"name":"bob"}`, "-rc", ".name")
	require.NoError(t, err)
	assert.Equal(t, "bob", strings.TrimSpace(out))
}

func TestJqCommand_PrettyPrintDefault(t *testing.T) {
	// Default output should be pretty-printed (indented)
	out, err := runJqCommand(t, `{"a":{"b":1}}`, ".")
	require.NoError(t, err)
	assert.Contains(t, out, "\n")
	assert.Contains(t, out, "  ") // indentation
}

// --- Property-based UTF-8 round-trip tests ---

// randUTF8String generates a random string containing a mix of ASCII and
// multi-byte UTF-8 characters. It draws from: ASCII printable, Latin
// extended, CJK, emoji, math symbols, and explicit problem characters
// (em-dash, zero-width space, BOM, etc.).
func randUTF8String(rng *rand.Rand, maxLen int) string {
	// Character pools covering various UTF-8 byte widths.
	pools := [][]rune{
		// 1-byte: ASCII printable (avoid backslash and quote to keep JSON simple)
		[]rune("abcdefghijklmnopqrstuvwxyz0123456789 !@#$%^&*()-=+[]{}|;:,.<>?/~`"),
		// 2-byte: Latin extended, Greek, Cyrillic
		[]rune("àáâãäåæçèéêëìíîïðñòóôõöùúûüýþÿαβγδεζ"),
		// 3-byte: CJK, punctuation, known problem characters
		[]rune("你好世界東京日本語한국어—–\u2018\u2019\u201C\u201D\u2022\u2026\u20AC\u00A3\u00A5\u00A9\u00AE\u2122\u00A7\u00B6\u2020\u2021\u200B\u200C\u200D\uFEFF"),
		// 4-byte: Emoji, math symbols
		[]rune("\U0001F389\U0001F680\U0001F4BB\U0001F525\u2705\u274C\U0001F3AF\U0001F3D7\U0001F4E6\U0001F9EA\U0001F52C\U0001F4CA\U0001F3AD\U0001F3EA"),
	}

	n := rng.Intn(maxLen) + 1
	var b strings.Builder
	for i := 0; i < n; i++ {
		pool := pools[rng.Intn(len(pools))]
		b.WriteRune(pool[rng.Intn(len(pool))])
	}
	return b.String()
}

func TestJqProperty_UTF8RoundTrip(t *testing.T) {
	// Property: for any valid UTF-8 string s, encoding s as a JSON object
	// field value then processing through "ddx jq '.'" must produce valid
	// JSON whose decoded field value equals the original string.
	rng := rand.New(rand.NewSource(42)) // deterministic seed
	const iterations = 500

	for i := 0; i < iterations; i++ {
		s := randUTF8String(rng, 200)
		if !utf8.ValidString(s) {
			continue // skip if generator produced invalid UTF-8
		}

		// Build input JSON with the random string as a value.
		input := map[string]interface{}{"text": s, "n": i}
		inputBytes, err := json.Marshal(input)
		require.NoError(t, err, "iteration %d: marshal input", i)

		// Run through ddx jq with identity filter.
		out, err := runJqCommand(t, string(inputBytes), "-c", ".")
		require.NoError(t, err, "iteration %d: jq failed for input %q", i, s)

		// Parse the output JSON.
		var result map[string]interface{}
		err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
		require.NoError(t, err, "iteration %d: output is not valid JSON: %q (input string: %q)", i, out, s)

		// The text field must round-trip exactly.
		assert.Equal(t, s, result["text"],
			"iteration %d: text field corrupted through jq", i)
	}
}

func TestJqProperty_UTF8MutationRoundTrip(t *testing.T) {
	// Property: for any valid UTF-8 string s, using ddx jq to mutate a
	// different field must not corrupt the string field.
	rng := rand.New(rand.NewSource(99))
	const iterations = 500

	for i := 0; i < iterations; i++ {
		s := randUTF8String(rng, 200)
		if !utf8.ValidString(s) {
			continue
		}

		input := map[string]interface{}{"text": s, "delay_ms": 100}
		inputBytes, err := json.Marshal(input)
		require.NoError(t, err)

		// Mutate a different field — this is the exact pattern from the bug report.
		out, err := runJqCommand(t, string(inputBytes), "-c", ".delay_ms = 0")
		require.NoError(t, err, "iteration %d: jq mutation failed for %q", i, s)

		var result map[string]interface{}
		err = json.Unmarshal([]byte(strings.TrimSpace(out)), &result)
		require.NoError(t, err, "iteration %d: output JSON invalid: %q", i, out)

		assert.Equal(t, s, result["text"],
			"iteration %d: text corrupted after mutating delay_ms", i)
		assert.Equal(t, float64(0), result["delay_ms"],
			"iteration %d: delay_ms not set to 0", i)
	}
}

func TestJqProperty_UTF8FileRoundTrip(t *testing.T) {
	// Property: write JSON to a file, process with ddx jq to a new file,
	// read back — the string must survive the full file I/O cycle.
	// This tests the exact pattern from the bug report:
	//   ddx jq '.delay_ms = 0' file.json > file.json.tmp && mv ...
	rng := rand.New(rand.NewSource(7))
	const iterations = 200
	dir := t.TempDir()

	for i := 0; i < iterations; i++ {
		s := randUTF8String(rng, 300)
		if !utf8.ValidString(s) {
			continue
		}

		input := map[string]interface{}{"text": s, "delay_ms": 100, "seq": i}
		inputBytes, err := json.Marshal(input)
		require.NoError(t, err)

		inFile := filepath.Join(dir, fmt.Sprintf("in-%d.json", i))
		require.NoError(t, os.WriteFile(inFile, inputBytes, 0644))

		// Process through ddx jq via file arg (not stdin).
		factory := NewCommandFactory(dir)
		root := factory.NewRootCommand()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs([]string{"jq", "-c", ".delay_ms = 0", inFile})
		require.NoError(t, root.Execute(), "iteration %d", i)

		// Write output to a new file and read it back (simulating redirect+mv).
		outFile := filepath.Join(dir, fmt.Sprintf("out-%d.json", i))
		require.NoError(t, os.WriteFile(outFile, buf.Bytes(), 0644))

		data, err := os.ReadFile(outFile)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(bytes.TrimSpace(data), &result)
		require.NoError(t, err, "iteration %d: file output not valid JSON: %q", i, string(data))

		assert.Equal(t, s, result["text"],
			"iteration %d: text corrupted through file round-trip", i)
	}
}
