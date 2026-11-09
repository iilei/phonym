// Command phonym encodes IDs as pronounceable names and decodes them back.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/iilei/phonym"
)

const (
	// defaultRelease is used when -release is not given.
	defaultRelease = "1.0.0+soft"

	cmdEncode   = "encode"
	cmdDecode   = "decode"
	cmdReleases = "releases"
	cmdVersion  = "version"
	cmdHelp     = "help"
	hexBase     = 16

	usage = `usage:
  phonym [encode] [-release LABEL] [-short] ID...   encode IDs (decimal, 0x, 0o, 0b)
  phonym decode [-release LABEL] NAME...           decode full or short forms
  phonym releases                                  list the registry
  phonym version                                   print the version

The default release is ` + defaultRelease + `. A short form needs -release to decode.
`
)

// Set by goreleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd := cmdEncode
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && isCommand(args[0]) {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case cmdEncode:
		err = encode(args, stdout, stderr)
	case cmdDecode:
		err = decode(args, stdout, stderr)
	case cmdReleases:
		listReleases(stdout)
	case cmdVersion:
		fmt.Fprintf(stdout, "phonym %s (%s, %s)\n", version, commit, date)
	case cmdHelp:
		fmt.Fprint(stdout, usage)
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "phonym:", err)
		return 1
	}
	return 0
}

func isCommand(s string) bool {
	switch s {
	case cmdEncode, cmdDecode, cmdReleases, cmdVersion, cmdHelp:
		return true
	}
	return false
}

func encode(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cmdEncode, flag.ContinueOnError)
	fs.SetOutput(stderr)
	release := fs.String("release", defaultRelease, "release label")
	short := fs.Bool("short", false, "print the short form (decoding it needs the release)")
	maxBits := fs.Int("max-bits", phonym.MaxBits, "largest ID size in bits")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("no ID given")
	}
	for _, arg := range fs.Args() {
		id, ok := new(big.Int).SetString(arg, 0)
		if !ok || id.Sign() < 0 {
			return fmt.Errorf("%q is not a non-negative integer", arg)
		}
		var name string
		var err error
		if *short {
			name, err = phonym.EncodeShort(id, *release, phonym.WithMaxBits(*maxBits))
		} else {
			name, err = phonym.Encode(id, *release, phonym.WithMaxBits(*maxBits))
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, name)
	}
	return nil
}

func decode(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cmdDecode, flag.ContinueOnError)
	fs.SetOutput(stderr)
	release := fs.String("release", "", "release of short forms (full forms carry their own)")
	hex := fs.Bool("hex", false, "print IDs in hex")
	maxBits := fs.Int("max-bits", phonym.MaxBits, "largest ID size in bits (and name length in letters)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("no name given")
	}
	for _, name := range fs.Args() {
		d, err := phonym.Decode(name, *release, phonym.WithMaxBits(*maxBits))
		if err != nil {
			return err
		}
		id := d.ID.String()
		if *hex {
			id = "0x" + d.ID.Text(hexBase)
		}
		form := "full"
		if d.Short {
			form = "short"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", id, d.Release, form)
	}
	return nil
}

func listReleases(stdout io.Writer) {
	for slot, r := range phonym.Releases() {
		status := ""
		switch {
		case r.Style() == nil:
			fmt.Fprintf(stdout, "%3d  (placeholder)\n", slot)
			continue
		case r.Retired:
			status = "  retired"
		}
		shape := "variable"
		if split := r.Style().Split; len(split) > 0 {
			shape = strings.Trim(strings.ReplaceAll(fmt.Sprint(split), " ", "+"), "[]")
			if r.Style().Grow {
				shape += "+..."
			}
		}
		line := fmt.Sprintf("%3d  %-14s  %-8s%s", slot, r.Label, shape, status)
		fmt.Fprintln(stdout, strings.TrimRight(line, " "))
	}
}
