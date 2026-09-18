package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Trimmed from StatSystem.cpp, keeping what trips a naive parse: statements before the ranged chain,
// the druid branch's inner switch with its own default, and form cases that aren't linear.
const statSystemSnippet = `
void Player::UpdateAttackPowerAndDamage(bool ranged)
{
    float val2 = 0.0f;
    if (ranged)
    {
        index = UNIT_FIELD_RANGED_ATTACK_POWER;

        if (IsClass(CLASS_HUNTER, CLASS_CONTEXT_STATS))
        {
            val2 = level * 2.0f + GetStat(STAT_AGILITY) - 10.0f;
        }
        else if (IsClass(CLASS_ROGUE, CLASS_CONTEXT_STATS) || IsClass(CLASS_WARRIOR, CLASS_CONTEXT_STATS))
        {
            val2 = level + GetStat(STAT_AGILITY) - 10.0f;
        }
        else if (IsClass(CLASS_DRUID, CLASS_CONTEXT_STATS))
        {
            switch (GetShapeshiftForm())
            {
            case FORM_CAT:
                val2 = 0.0f;
                break;
            default:
                val2 = GetStat(STAT_AGILITY) - 10.0f;
                break;
            }
        }
        else
        {
            val2 = GetStat(STAT_AGILITY) - 10.0f;
        }
    }
    else
    {
        if (IsClass(CLASS_PALADIN, CLASS_CONTEXT_STATS) || IsClass(CLASS_DEATH_KNIGHT, CLASS_CONTEXT_STATS) || IsClass(CLASS_WARRIOR, CLASS_CONTEXT_STATS))
        {
            val2 = level * 3.0f + GetStat(STAT_STRENGTH) * 2.0f - 20.0f;
        }
        else if (IsClass(CLASS_HUNTER, CLASS_CONTEXT_STATS) || IsClass(CLASS_SHAMAN, CLASS_CONTEXT_STATS) || IsClass(CLASS_ROGUE, CLASS_CONTEXT_STATS))
        {
            val2 = level * 2.0f + GetStat(STAT_STRENGTH) + GetStat(STAT_AGILITY) - 20.0f;
        }
        else if (IsClass(CLASS_DRUID, CLASS_CONTEXT_STATS))
        {
            if (IsInFeralForm())
            {
                switch (aurEff->GetEffIndex())
                {
                case 0: // Predatory Strikes (effect 0)
                    mLevelMult = CalculatePct(1.0f, aurEff->GetAmount());
                    break;
                default:
                    break;
                }
            }

            switch (GetShapeshiftForm())
            {
            case FORM_CAT:
                val2 = (GetLevel() * mLevelMult) + GetStat(STAT_STRENGTH) * 2.0f + GetStat(STAT_AGILITY) - 20.0f + weapon_bonus + m_baseFeralAP;
                break;
            default:
                val2 = GetStat(STAT_STRENGTH) * 2.0f - 20.0f;
                break;
            }
        }
        else if (IsClass(CLASS_MAGE, CLASS_CONTEXT_STATS) || IsClass(CLASS_PRIEST, CLASS_CONTEXT_STATS) || IsClass(CLASS_WARLOCK, CLASS_CONTEXT_STATS))
        {
            val2 = GetStat(STAT_STRENGTH) - 10.0f;
        }
    }

    if (ranged)
    {
        val2 = 1.0f;
    }
}

const float m_diminishing_k[MAX_CLASSES] =
{
    0.9560f,  // Warrior
    0.9560f,  // Paladin
    0.9880f,  // Hunter
    0.9880f,  // Rogue
    0.9830f,  // Priest
    0.9560f,  // DK
    0.9880f,  // Shaman
    0.9830f,  // Mage
    0.9830f,  // Warlock
    0.0f,     // ??
    0.9720f   // Druid
};

    const float crit_to_dodge[MAX_CLASSES] =
    {
        0.85f / 1.15f,  // Warrior
        1.00f / 1.15f,  // Paladin
        1.11f / 1.15f,  // Hunter
        2.00f / 1.15f,  // Rogue
        1.00f / 1.15f,  // Priest
        0.85f / 1.15f,  // DK
        1.60f / 1.15f,  // Shaman
        1.00f / 1.15f,  // Mage
        0.97f / 1.15f,  // Warlock (?)
        0.0f,           // ??
        2.00f / 1.15f   // Druid
    };
`

func classID(t *testing.T, name string) int {
	t.Helper()
	c, ok := classByName(name)
	if !ok {
		t.Fatalf("no class %s", name)
	}
	return c.serverID
}

func checkAttackPower(t *testing.T, src string) {
	melee, ranged, err := parseAttackPower(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		class         string
		melee, ranged linearFormula
	}{
		{"WARRIOR", linearFormula{3, 2, 0, -20}, linearFormula{1, 0, 1, -10}},
		{"PALADIN", linearFormula{3, 2, 0, -20}, linearFormula{0, 0, 1, -10}},
		{"DEATH_KNIGHT", linearFormula{3, 2, 0, -20}, linearFormula{0, 0, 1, -10}},
		{"HUNTER", linearFormula{2, 1, 1, -20}, linearFormula{2, 0, 1, -10}},
		{"ROGUE", linearFormula{2, 1, 1, -20}, linearFormula{1, 0, 1, -10}},
		{"SHAMAN", linearFormula{2, 1, 1, -20}, linearFormula{0, 0, 1, -10}},
		{"DRUID", linearFormula{0, 2, 0, -20}, linearFormula{0, 0, 1, -10}},
		{"MAGE", linearFormula{0, 1, 0, -10}, linearFormula{0, 0, 1, -10}},
		{"WARLOCK", linearFormula{0, 1, 0, -10}, linearFormula{0, 0, 1, -10}},
	} {
		id := classID(t, tc.class)
		if melee[id] != tc.melee {
			t.Errorf("%s melee: got %+v, want %+v", tc.class, melee[id], tc.melee)
		}
		if ranged[id] != tc.ranged {
			t.Errorf("%s ranged: got %+v, want %+v", tc.class, ranged[id], tc.ranged)
		}
	}
}

func checkClassArrays(t *testing.T, statSystem, player string) {
	k, err := parseClassArray(statSystem, "m_diminishing_k")
	if err != nil {
		t.Fatal(err)
	}
	if k[classID(t, "WARRIOR")-1] != 0.956 || k[classID(t, "HUNTER")-1] != 0.988 || k[classID(t, "DRUID")-1] != 0.972 {
		t.Errorf("m_diminishing_k: %v", k)
	}
	c2d, err := parseClassArray(player, "crit_to_dodge")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c2d[classID(t, "ROGUE")-1], float32(2.0)/float32(1.15); got != want {
		t.Errorf("rogue crit_to_dodge: got %v, want %v", got, want)
	}
}

func TestParseSnippet(t *testing.T) {
	checkAttackPower(t, statSystemSnippet)
	checkClassArrays(t, statSystemSnippet, statSystemSnippet)
}

func TestParseLinear(t *testing.T) {
	for expr, want := range map[string]linearFormula{
		"level * 3.0f + GetStat(STAT_STRENGTH) * 2.0f - 20.0f": {3, 2, 0, -20},
		"0.0f":                                  {},
		"GetStat(STAT_AGILITY) - 10.0f":         {0, 0, 1, -10},
		"-10.0f + 2 * level":                    {2, 0, 0, -10},
		"level + GetStat(STAT_AGILITY) - 10.0f": {1, 0, 1, -10},
	} {
		got, err := parseLinear(expr)
		if err != nil {
			t.Errorf("%q: %v", expr, err)
		} else if got != want {
			t.Errorf("%q: got %+v, want %+v", expr, got, want)
		}
	}
	for _, expr := range []string{"(GetLevel() * mLevelMult) + 1.0f", "level * level", "GetStat(STAT_STAMINA)"} {
		if _, err := parseLinear(expr); err == nil {
			t.Errorf("%q: expected an error", expr)
		}
	}
}

// TestParseServerSources runs the parsers on the AzerothCore checkout, when there is one (dock.sh
// mounts it at /ac).
func TestParseServerSources(t *testing.T) {
	dir := os.Getenv("AC_DIR")
	if dir == "" {
		dir = "/ac"
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Skipf("no AzerothCore checkout: %v", err)
		}
		return string(b)
	}
	unitH := read("src/server/game/Entities/Unit/Unit.h")
	statSystem := read("src/server/game/Entities/Unit/StatSystem.cpp")
	player := read("src/server/game/Entities/Player/Player.cpp")

	ratings, err := parseCombatRatings(unitH)
	if err != nil {
		t.Fatal(err)
	}
	if len(ratings) != 25 || ratings[24].Name != "ARMOR_PENETRATION" || ratings[17].Name != "HASTE_MELEE" {
		t.Errorf("CombatRating enum: %+v", ratings)
	}
	checkAttackPower(t, statSystem)
	checkClassArrays(t, statSystem, player)
	for _, name := range []string{"miss_cap", "parry_cap", "dodge_cap"} {
		if _, err := parseClassArray(statSystem, name); err != nil {
			t.Error(err)
		}
	}
	if _, err := parseClassArray(player, "dodge_base"); err != nil {
		t.Error(err)
	}
}
