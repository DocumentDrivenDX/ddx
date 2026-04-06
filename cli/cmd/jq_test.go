package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
