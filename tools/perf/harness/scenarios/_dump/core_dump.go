package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// perfDumpRequest writes a benchmark's request to $PERF_DUMP_DIR as sim_<package>.json, with the
// root sim package's raid as sim_raid.json. go test runs each test binary in its package's directory.
func perfDumpRequest(rsr *proto.RaidSimRequest) {
	dir := os.Getenv("PERF_DUMP_DIR")
	if dir == "" {
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	wd = filepath.ToSlash(wd)
	name := "raid"
	if i := strings.LastIndex(wd, "/sim/"); i >= 0 {
		name = strings.ReplaceAll(wd[i+len("/sim/"):], "/", "_")
	}
	data, err := protojson.Marshal(rsr)
	if err != nil {
		panic(err)
	}
	// protojson varies its whitespace on purpose; compacting keeps a rerun byte for byte the same
	var out bytes.Buffer
	if err := json.Compact(&out, data); err != nil {
		panic(err)
	}
	out.WriteByte('\n')
	if err := os.WriteFile(filepath.Join(dir, "sim_"+name+".json"), out.Bytes(), 0666); err != nil {
		panic(err)
	}
}
