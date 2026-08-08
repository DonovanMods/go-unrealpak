// Command unrealpak inspects and builds Unreal Engine v11 .pak archives.
//
// It is deliberately game-agnostic: mount points and entry paths are
// whatever the caller supplies, and no game's conventions are baked in.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "unrealpak:", err)
		os.Exit(1)
	}
}

const usage = `unrealpak - inspect and build Unreal Engine v11 .pak archives

usage:
  unrealpak info    <pak>
  unrealpak list    <pak> [--json]
  unrealpak cat     <pak> <path>
  unrealpak extract <pak> <dir> [--filter <glob>]
  unrealpak build   <dir> <pak> [--mount <mountpoint>]
`

// run is the testable entry point: every subcommand writes its results to
// out rather than os.Stdout so tests can drive the real dispatch path.
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("no subcommand given\n\n%s", usage)
	}
	switch args[0] {
	case "info":
		return cmdInfo(args[1:], out)
	case "list":
		return cmdList(args[1:], out)
	case "cat":
		return cmdCat(args[1:], out)
	case "extract":
		return cmdExtract(args[1:], out)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q\n\n%s", args[0], usage)
	}
}
