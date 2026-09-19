package optimizer

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// TestReplayFixtures checks requests the UI's pool builder made (ui/core/optimizer/README.md) against
// the equip rules: each one must prepare, compile, and hold a seed that passes CheckGear. It doesn't
// run the search. Fixtures are .json (as the tab exports them) or .json.gz.
func TestReplayFixtures(t *testing.T) {
	var files []string
	for _, pattern := range []string{"testdata/replay/*.json", "testdata/replay/*.json.gz"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures in testdata/replay")
	}

	phases := map[int32]bool{}
	for _, file := range files {
		req := &proto.OptimizeGearRequest{}
		data, err := readFixture(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if err := protojson.Unmarshal(data, req); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		phases[req.GetSettings().GetContentPhase()] = true

		t.Run(filepath.Base(file), func(t *testing.T) {
			r, err := PrepareRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			if r.Pool.CatalogDate == "" {
				t.Error("the pool has no catalog date")
			}
			pool, err := CompilePool(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := pool.CheckGear(r.Seed); err != nil {
				t.Fatalf("seed: %v", err)
			}
			// an empty off hand is fine for two-hander specs
			for slot, candidates := range pool.Slots {
				if len(candidates) == 0 && !pool.Locked[slot] && proto.ItemSlot(slot) != proto.ItemSlot_ItemSlotOffHand {
					t.Errorf("%s has no candidates", proto.ItemSlot(slot))
				}
			}
			if len(pool.Gems) == 0 {
				t.Error("the pool has no gems")
			}
		})
	}

	for phase := int32(1); phase <= 5; phase++ {
		if !phases[phase] {
			t.Errorf("no fixture for content phase %d", phase)
		}
	}
}

func readFixture(file string) ([]byte, error) {
	data, err := os.ReadFile(file)
	if err != nil || !strings.HasSuffix(file, ".gz") {
		return data, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
