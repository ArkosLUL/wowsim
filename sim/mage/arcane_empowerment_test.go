package mage

import (
	"math"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
)

// ArcaneTalents with this many points of Arcane Empowerment, the arcane tree's 21st talent
func arcaneTalentsWithEmpowerment(points byte) string {
	trees := strings.SplitN(ArcaneTalents, "-", 2)
	arcane := []byte(trees[0])
	arcane[20] = '0' + points
	return string(arcane) + "-" + trees[1]
}

// mod-spell-tweaks' Arcane Empowerment (31579, 31582, 31583) adds 5 a point to SPELLMOD_BONUS_MULTIPLIER,
// in hundredths of Arcane Blast's and the Arcane Missiles missile's spell power coefficient.
func TestArcaneEmpowermentCoefficients(t *testing.T) {
	for points := range byte(4) {
		_, mage, _ := gearlessSim(t, arcaneTalentsWithEmpowerment(points), PlayerOptionsArcane, nil)
		if got := mage.Talents.ArcaneEmpowerment; got != int32(points) {
			t.Fatalf("%d points of Arcane Empowerment, want %d", got, points)
		}
		for _, c := range []struct {
			name  string
			spell *core.Spell
			stock float64
		}{
			{"Arcane Blast", mage.ArcaneBlast, 0.714},
			{"Arcane Missiles", mage.ArcaneMissilesTickSpell, 0.2857},
		} {
			if got, want := c.spell.Direct.SP, c.stock+0.05*float64(points); math.Abs(got-want) > 1e-9 {
				t.Errorf("%s with %d points of Arcane Empowerment: coefficient %v, want %v", c.name, points, got, want)
			}
		}
	}
}
