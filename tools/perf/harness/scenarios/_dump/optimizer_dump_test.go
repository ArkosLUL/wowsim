//go:build optimizer_slow

package optimizer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

// TestPerfDumpSlowRequests writes the slow suite's requests to $PERF_DUMP_DIR as
// opt_<spec>_p<phase>.json, at Quick.
func TestPerfDumpSlowRequests(t *testing.T) {
	dir := os.Getenv("PERF_DUMP_DIR")
	if dir == "" {
		t.Skip("PERF_DUMP_DIR isn't set")
	}
	for _, c := range slowCases {
		req := slowRequest(t, c, proto.OptimizerEffort_OptimizerEffortQuick)
		for i, row := range req.Pool.CatalogItems {
			// the search never reads sources, and they're most of the file
			row = goproto.Clone(row).(*proto.CatalogItem)
			row.Sources = nil
			req.Pool.CatalogItems[i] = row
		}
		data, err := protojson.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := json.Compact(&out, data); err != nil {
			t.Fatal(err)
		}
		out.WriteByte('\n')
		name := fmt.Sprintf("opt_%s_p%d.json", c.spec, c.phase)
		if err := os.WriteFile(filepath.Join(dir, name), out.Bytes(), 0666); err != nil {
			t.Fatal(err)
		}
	}
}
