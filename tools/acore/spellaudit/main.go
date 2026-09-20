// Command spellaudit exports what the live server knows about every spell the sim
// names, so a class work item can review its own spells against the server rather
// than against Classic research.
//
// It joins three sources: the spell ids the sim spells out (the same scan
// tools/acore/spellids does), the committed `.simval spelldump` capture, and the
// generated sim/core/serverdata tables. The result is three CSVs under
// docs/azerothcore-parity/audit, plus a summary per sim area on stdout.
//
//	tools/acore/dock.sh run ./tools/acore/spellaudit
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

func main() {
	root := flag.String("root", ".", "the checkout to scan")
	dump := flag.String("dump", spellset.DefaultDumpPath, "the committed spell capture")
	out := flag.String("out", "docs/azerothcore-parity/audit", "where the CSVs are written")
	area := flag.String("area", "", "only report this sim area, e.g. hunter or common/wotlk")
	flag.Parse()

	if err := run(*root, *dump, *out, *area); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, dumpPath, outDir, area string) error {
	capture, err := spellset.ReadDump(dumpPath)
	if err != nil {
		return err
	}
	if len(capture) == 0 {
		return fmt.Errorf("%s holds no spells; capture one first (tools/acore/spellids README)", dumpPath)
	}

	literals, err := spellset.ScanSim(root)
	if err != nil {
		return err
	}

	audit := buildAudit(literals, capture, area)
	if err := audit.write(outDir); err != nil {
		return err
	}
	audit.summarize(os.Stdout)
	return nil
}
