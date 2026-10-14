package main

import "testing"

// A CANT_CRIT spell with a crit capable effect never crits on the server; a capture taken before the
// module's dump read the attribute says it does.
func TestCritCapableReadsCantCrit(t *testing.T) {
	for _, id := range []int32{
		5938,  // Shiv: weapon damage
		25742, // Seal of Righteousness: school damage
		52042, // Healing Stream Totem's heal
	} {
		if capable, known := critCapable(id); !known || capable {
			t.Errorf("spell %d: capable %v known %v, want a known spell that can't crit", id, capable, known)
		}
	}
}
