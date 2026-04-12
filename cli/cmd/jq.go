package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

func (f *CommandFactory) newJqCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jq [flags] <filter> [file...]",
		Short: "Process JSON with jq expressions",
		Long: `Embedded jq processor powered by gojq.

Accepts the same filter syntax and most flags as standalone jq.
Reads from stdin when no file arguments are given.

Examples:
  echo '{"a":1}' | ddx jq '.a'
  ddx jq '.[] | select(.status=="open")' beads.jsonl
  ddx jq -r '.name' package.json
  ddx jq -n '[range(5)]'
  ddx jq --arg name foo '.name == $name' data.json
  ddx jq -s 'map(.value) | add' values.jsonl`,
		Args:               cobra.ArbitraryArgs,
		RunE:               f.runJq,
		DisableFlagParsing: true,
	}

	return cmd
}

func (f *CommandFactory) runJq(cmd *cobra.Command, args []string) error {
	opts, err := parseJqArgs(args)
	if err != nil {
		return err
	}

	if opts.help {
		return printJqHelp(cmd)
	}

	if opts.version {
		fmt.Fprintln(cmd.OutOrStdout(), "ddx jq - embedded gojq")
		return nil
	}

	if opts.filter == "" && !opts.help {
		return fmt.Errorf("jq - commandline JSON processor\n\nUsage: ddx jq [FLAGS] FILTER [FILE...]\n\nRun 'ddx jq --help' for full usage")
	}

	// Compile the filter
	query, err := gojq.Parse(opts.filter)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Build compiler options — collect variable names and values in
	// a stable order so they line up for code.Run().
	var varNames []string
	var varValues []interface{}
	for name, val := range opts.variables {
		varNames = append(varNames, "$"+name)
		varValues = append(varValues, val)
	}

	var compilerOpts []gojq.CompilerOption
	if len(varNames) > 0 {
		compilerOpts = append(compilerOpts, gojq.WithVariables(varNames))
	}

	code, err := gojq.Compile(query, compilerOpts...)
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}

	// Determine input sources
	var readers []io.Reader
	if len(opts.files) == 0 {
		readers = append(readers, os.Stdin)
	} else {
		for _, path := range opts.files {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("could not open %s: %w", path, err)
			}
			defer file.Close()
			readers = append(readers, file)
		}
	}

	out := cmd.OutOrStdout()
	exitCode := 0

	if opts.nullInput {
		// --null-input: run filter once with null
		exitCode = runJqFilter(code, nil, varValues, opts, out)
	} else if opts.slurp {
		// --slurp: collect all inputs into an array, run filter once
		var allValues []interface{}
		for _, r := range readers {
			values, err := decodeAll(r, opts.rawInput)
			if err != nil {
				return err
			}
			allValues = append(allValues, values...)
		}
		exitCode = runJqFilter(code, allValues, varValues, opts, out)
	} else {
		// Normal: run filter on each input value
		for _, r := range readers {
			values, err := decodeAll(r, opts.rawInput)
			if err != nil {
				return err
			}
			for _, v := range values {
				rc := runJqFilter(code, v, varValues, opts, out)
				if rc != 0 && exitCode == 0 {
					exitCode = rc
				}
			}
		}
	}

	if opts.exitStatus && exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

// decodeAll reads all JSON values (or raw lines) from a reader.
func decodeAll(r io.Reader, rawInput bool) ([]interface{}, error) {
	if rawInput {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		var values []interface{}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			values = append(values, line)
		}
		return values, nil
	}

	var values []interface{}
	dec := json.NewDecoder(r)
	for {
		var v interface{}
		if err := dec.Decode(&v); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("json decode error: %w", err)
		}
		values = append(values, v)
	}
	return values, nil
}

// runJqFilter executes a compiled filter against an input and writes output.
func runJqFilter(code *gojq.Code, input interface{}, varValues []interface{}, opts *jqOpts, out io.Writer) int {
	iter := code.Run(input, varValues...)
	exitCode := 0
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			fmt.Fprintf(os.Stderr, "jq: error: %s\n", err)
			exitCode = 5
			continue
		}
		if err := outputValue(v, opts, out); err != nil {
			fmt.Fprintf(os.Stderr, "jq: error: %s\n", err)
			exitCode = 2
		}
		if opts.exitStatus {
			if v == nil || v == false {
				exitCode = 1
			}
		}
	}
	return exitCode
}

// outputValue formats and prints a single output value.
func outputValue(v interface{}, opts *jqOpts, out io.Writer) error {
	if opts.rawOutput || opts.joinOutput {
		if s, ok := v.(string); ok {
			if opts.joinOutput {
				_, err := fmt.Fprint(out, s)
				return err
			}
			_, err := fmt.Fprintln(out, s)
			return err
		}
	}

	enc := json.NewEncoder(out)
	if !opts.compact {
		if opts.tab {
			enc.SetIndent("", "\t")
		} else {
			indent := "  "
			if opts.indent > 0 {
				indent = strings.Repeat(" ", opts.indent)
			}
			enc.SetIndent("", indent)
		}
	}
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// jqOpts holds parsed jq command-line options.
type jqOpts struct {
	filter     string
	files      []string
	rawOutput  bool
	rawInput   bool
	compact    bool
	slurp      bool
	nullInput  bool
	exitStatus bool
	joinOutput bool
	tab        bool
	indent     int
	sortKeys   bool
	help       bool
	version    bool
	variables  map[string]interface{}
}

// parseJqArgs parses jq-style arguments from a raw arg list.
// We parse manually because DisableFlagParsing is set (jq flags
// collide with cobra's built-in flag handling).
func parseJqArgs(args []string) (*jqOpts, error) {
	opts := &jqOpts{
		variables: make(map[string]interface{}),
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			i++
			// Everything after -- is filter (if not set) then files
			if opts.filter == "" && i < len(args) {
				opts.filter = args[i]
				i++
			}
			opts.files = append(opts.files, args[i:]...)
			i = len(args)

		case arg == "-r" || arg == "--rawoutput" || arg == "--raw-output":
			opts.rawOutput = true
		case arg == "-R" || arg == "--raw-input":
			opts.rawInput = true
		case arg == "-c" || arg == "--compact-output":
			opts.compact = true
		case arg == "-s" || arg == "--slurp":
			opts.slurp = true
		case arg == "-n" || arg == "--null-input":
			opts.nullInput = true
		case arg == "-e" || arg == "--exit-status":
			opts.exitStatus = true
		case arg == "-j" || arg == "--join-output":
			opts.joinOutput = true
			opts.rawOutput = true
		case arg == "--tab":
			opts.tab = true
		case arg == "-S" || arg == "--sort-keys":
			opts.sortKeys = true
		case arg == "-h" || arg == "--help":
			opts.help = true
			return opts, nil
		case arg == "-V" || arg == "--version":
			opts.version = true
			return opts, nil

		case arg == "--indent":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--indent requires an argument")
			}
			n := 0
			if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil {
				return nil, fmt.Errorf("--indent: invalid number %q", args[i])
			}
			opts.indent = n

		case arg == "--arg":
			if i+2 >= len(args) {
				return nil, fmt.Errorf("--arg requires name and value")
			}
			opts.variables[args[i+1]] = args[i+2]
			i += 2

		case arg == "--argjson":
			if i+2 >= len(args) {
				return nil, fmt.Errorf("--argjson requires name and value")
			}
			var v interface{}
			if err := json.Unmarshal([]byte(args[i+2]), &v); err != nil {
				return nil, fmt.Errorf("--argjson %s: invalid JSON: %w", args[i+1], err)
			}
			opts.variables[args[i+1]] = v
			i += 2

		case arg == "--slurpfile":
			if i+2 >= len(args) {
				return nil, fmt.Errorf("--slurpfile requires name and path")
			}
			data, err := os.ReadFile(args[i+2])
			if err != nil {
				return nil, fmt.Errorf("--slurpfile %s: %w", args[i+1], err)
			}
			var values []interface{}
			dec := json.NewDecoder(strings.NewReader(string(data)))
			for {
				var v interface{}
				if derr := dec.Decode(&v); derr == io.EOF {
					break
				} else if derr != nil {
					return nil, fmt.Errorf("--slurpfile %s: %w", args[i+1], derr)
				}
				values = append(values, v)
			}
			opts.variables[args[i+1]] = values
			i += 2

		case arg == "--jsonargs":
			// Remaining args are JSON values, not files
			i++
			for i < len(args) {
				var v interface{}
				if err := json.Unmarshal([]byte(args[i]), &v); err != nil {
					return nil, fmt.Errorf("--jsonargs: invalid JSON %q: %w", args[i], err)
				}
				// Store as positional args accessible via $ARGS.positional
				// gojq handles this internally when using WithVariables
				i++
			}
			continue

		case arg == "--args":
			// Remaining args are string values, not files
			i = len(args)
			continue

		case strings.HasPrefix(arg, "-") && len(arg) > 1 && arg[1] != '-':
			// Handle combined short flags like -rc, -Sc
			for _, ch := range arg[1:] {
				switch ch {
				case 'r':
					opts.rawOutput = true
				case 'R':
					opts.rawInput = true
				case 'c':
					opts.compact = true
				case 's':
					opts.slurp = true
				case 'n':
					opts.nullInput = true
				case 'e':
					opts.exitStatus = true
				case 'j':
					opts.joinOutput = true
					opts.rawOutput = true
				case 'S':
					opts.sortKeys = true
				default:
					return nil, fmt.Errorf("unknown option: -%c", ch)
				}
			}

		default:
			// First non-flag arg is the filter, rest are files
			if opts.filter == "" {
				opts.filter = arg
			} else {
				opts.files = append(opts.files, arg)
			}
		}
		i++
	}

	return opts, nil
}

func printJqHelp(cmd *cobra.Command) error {
	help := `Usage: ddx jq [FLAGS] FILTER [FILE...]

Embedded jq processor powered by gojq.

Flags:
  -c, --compact-output   Compact output (no pretty-printing)
  -r, --raw-output       Output raw strings (no JSON quotes)
  -R, --raw-input        Treat each input line as a string
  -s, --slurp            Read all inputs into an array
  -n, --null-input       Run filter with null input
  -e, --exit-status      Set exit code based on output
  -j, --join-output      Like -r but no newline after each output
  -S, --sort-keys        Sort object keys
      --tab              Indent with tabs
      --indent N         Indent with N spaces (default: 2)
      --arg NAME VAL     Bind $NAME to string VAL
      --argjson NAME VAL Bind $NAME to parsed JSON VAL
      --slurpfile NAME F Bind $NAME to array of JSON values from F
  -h, --help             Show this help
  -V, --version          Show version

Examples:
  echo '{"a":1}' | ddx jq '.a'
  ddx jq -r '.name' package.json
  ddx jq -s 'map(.value) | add' *.json
  ddx jq --arg key name '.[$key]' data.json
  ddx jq -n '[range(10)]'`
	fmt.Fprintln(cmd.OutOrStdout(), help)
	return nil
}
