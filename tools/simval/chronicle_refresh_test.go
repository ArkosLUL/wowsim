package main

import (
	"fmt"
	"strings"
	"testing"
)

// buildRefreshLog writes minimal SPELL_DAMAGE and SPELL_AURA_APPLIED lines for one
// caster/target pair, enough for pairRefreshes: it never reads CHRONICLE_UNIT_INFO,
// since a base-params event already carries its own source and destination names.
func buildRefreshLog(t *testing.T, lines ...string) *chronicleLog {
	t.Helper()
	log, err := parseChronicleLog("test", strings.NewReader(strings.Join(lines, "\n")+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func critLine(ms int64, spellID int32, name string, amount int64) string {
	return fmt.Sprintf(`%d  SPELL_DAMAGE,0x1,"Testmage",0x511,0xF1,"Dummy",0xa28,%d,"%s",0x4,%d,0,4,0,0,0,1,nil,nil`,
		ms, spellID, name, amount)
}

func refreshLine(ms int64, auraID int32, name string) string {
	return fmt.Sprintf(`%d  SPELL_AURA_APPLIED,0x1,"Testmage",0x511,0xF1,"Dummy",0xa28,%d,"%s",0x4,BUFF`,
		ms, auraID, name)
}

// A later crit landing before an earlier crit's own refresh must not make that earlier
// crit unclaimable: each crit draws its own delay, so refreshes don't always arrive in
// crit order. This is the bug pairRefreshes's oldest-first pending list fixes over
// simply tracking the single latest crit.
func TestPairRefreshesOutOfOrderLanding(t *testing.T) {
	log := buildRefreshLog(t,
		critLine(1000, 111, "FeederA", 100),
		critLine(1100, 222, "FeederB", 200),
		refreshLine(1150, 999, "Ignite"), // 50ms after B, 150ms after A: B is closer
		refreshLine(1390, 999, "Ignite"), // B already claimed; this is A's own refresh
	)

	pairs, unpaired, excess := pairRefreshes(log, "Testmage", refreshOptions{
		AuraSpellID: 999, Feeders: []int32{111, 222}, WindowMs: 500,
	})
	if len(unpaired) != 0 || excess != 0 {
		t.Fatalf("unpaired=%v excess=%d, want both empty", unpaired, excess)
	}
	if len(pairs) != 2 {
		t.Fatalf("%d pairs, want 2: %+v", len(pairs), pairs)
	}
	if pairs[0].FeederSpellID != 222 || pairs[0].delayMs() != 50 {
		t.Errorf("first pair = %+v, want feeder 222 delay 50ms", pairs[0])
	}
	if pairs[1].FeederSpellID != 111 || pairs[1].delayMs() != 390 {
		t.Errorf("second pair = %+v, want feeder 111 delay 390ms", pairs[1])
	}
}

// A crit outside every refresh's window is reported as excess, not silently dropped.
func TestPairRefreshesExcessCrit(t *testing.T) {
	log := buildRefreshLog(t,
		critLine(1000, 111, "FeederA", 100),
		refreshLine(2000, 999, "Ignite"), // 1000ms later: outside the 500ms window
	)

	pairs, unpaired, excess := pairRefreshes(log, "Testmage", refreshOptions{
		AuraSpellID: 999, Feeders: []int32{111}, WindowMs: 500,
	})
	if len(pairs) != 0 {
		t.Fatalf("%d pairs, want 0: %+v", len(pairs), pairs)
	}
	if len(unpaired) != 1 || unpaired[0] != 2000 {
		t.Errorf("unpaired = %v, want [2000]", unpaired)
	}
	if excess != 1 {
		t.Errorf("excess = %d, want 1", excess)
	}
}

// A refresh outside every remaining crit's window is unpaired, and stays unpaired on a
// second refresh rather than reusing a crit already ruled too old.
func TestPairRefreshesUnpairedApplication(t *testing.T) {
	log := buildRefreshLog(t,
		refreshLine(1000, 999, "Ignite"), // no crit came before it at all
	)

	pairs, unpaired, excess := pairRefreshes(log, "Testmage", refreshOptions{
		AuraSpellID: 999, Feeders: []int32{111}, WindowMs: 500,
	})
	if len(pairs) != 0 || excess != 0 {
		t.Fatalf("pairs=%v excess=%d, want both empty", pairs, excess)
	}
	if len(unpaired) != 1 {
		t.Errorf("unpaired = %v, want 1 entry", unpaired)
	}
}
