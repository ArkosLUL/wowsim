# Healer sims: Holy Paladin, Resto Druid and Resto Shaman pages that heal

Status: approved by the user (2026-10-23); HL1 next.

## Context

- Since CAP-UX2 ([caps PLAN](../caps-and-priority/caps-and-priority.PLAN.md)), a character whose main talent tree is
  a healing tree loads only on a healer page.
- Holy Paladin, Resto Druid and Resto Shaman pages exist but are `LaunchStatus.Unlaunched`
  (`ui/core/launched_sims.ts`) and cast nothing:
  - `sim/paladin/holy/rotation.go` waits 5 s per GCD;
  - Resto Druid and Resto Shaman have an empty `autoRotation` and `rotations: []`.
- Healing Priest (`ui/healing_priest`, `sim/priest/healing`) is the one working healer page, at Alpha. Its APLs
  (`ui/healing_priest/apls`) cap each spell with `spellCpm < N` and never fill every GCD.
- The goal: each of the four healer pages in the class menu, with a default rotation that heals, per-phase
  presets, and the Caps tab's "Measure at my gear" (`sim/statslopes`, healer role in HPS) giving values that
  drive reforge suggestions.

**Heal model as it is:**
- Healer raids get 9 target dummies (`ui/core/individual_sim_ui.ts`; 1 in tests via `IsHealer`). They're
  10000 HP units with no health bar, and nothing damages them.
- HPS is raw healing plus shielding, overheal included (`sim/core/spell_result.go`,
  `metrics_aggregator.go`).
- Prayer of Mending auto-procs every 5 s (`sim/priest/prayer_of_mending.go`) as its stand-in for incoming damage.
- Chain Heal never jumps (`chainHealJumps` in `sim/shaman/chain_heal.go` wants hurt units with a health bar).
  Earth Shield never procs (it waits for `OnSpellHitTaken`).
- Golden StatWeights rows read DPS weights only (`sim/core/test_suite.go`), so no healer stat value is guarded.
- `sim/statslopes/statslopes_test.go`'s `request()` adds a dummy only for Healing Priest.

**What each class has** (on master `a98c26b7c`):
- **Resto Shaman:** Riptide, Chain Heal, Healing Wave, Lesser Healing Wave, Earth Shield, Ancestral Awakening,
  Healing Stream Totem, Mana Tide, Nature's Swiftness, Tidal Waves, Earthliving and Water Shield (a flat 100 MP5)
  are registered (`sim/shaman/heals.go`), several with parity tests.
  - Missing: Tidal Force, Healing Way, the glyphs of Healing Wave, Riptide, Water Shield and Mana Tide; T7 to T10
    resto sets; Riptide, Healing Wave and Earth Shield totems.
  - Riptide and Earth Shield register without checking the talent. `earth_shield_p_p_m` (`proto/shaman.proto`)
    is read nowhere.
- **Holy Paladin:** no heal spell.
  - Missing: Holy Light, Flash of Light, Holy Shock, Beacon of Light, Sacred Shield, Divine Illumination,
    Divine Favor, Infusion of Light, Light's Grace, Illumination, Judgements of the Pure, Healing Light,
    Holy Guidance, Holy Power, Seal of Wisdom, Seal of Light; holy glyphs (Holy Light, Beacon only in tests);
    T7 to T10 holy sets; healing librams.
  - Present: Divine Plea, Judgement of Light and Wisdom, Judgements of the Wise, Divine Intellect, Divinity,
    Spiritual Attunement, Lay on Hands, Benediction.
- **Resto Druid:** no heal spell.
  - Missing: Rejuvenation, Regrowth, Lifebloom, Nourish, Wild Growth, Swiftmend, Healing Touch, Tree of Life
    (only a form flag in `sim/druid/forms.go`), Living Seed, Revitalize, Improved Rejuvenation, Gift of Nature,
    Empowered Touch, Empowered Rejuvenation, Nature's Bounty, Tranquil Spirit, Improved Tree of Life; resto
    glyphs (only in tests); T7 to T10 resto sets (the sim's Dreamwalker and Nightsong are balance); healing idols.
  - Present: Nature's Grace (reads heals via `OnHealDealt`), Omen of Clarity, Nature's Swiftness, Innervate,
    Living Spirit, Natural Perfection, Intensity. Druid heals need `SpellFlagNaturesGrace` and
    `SpellFlagOmenTrigger` ([e2e-coverage PLAN](../e2e-coverage/e2e-coverage.PLAN.md)).
- **Server data:** every heal id is in `assets/db_inputs/acore/spelldump.jsonl`; `sim/core/serverdata/spells_auto_gen.go`
  gains them by rerunning `tools/acore/gen_serverdata` (SELECTs the live world DB) once the sim names them. Missing
  from the dump, so a live capture (`tools/acore/spellids/README.md#capturing`): 48503, 48504 (Living Seed), 20272
  (Illumination), 48540, 48542, 48543 (Revitalize).
- **UI:** all three pages have `sim.ts`, `presets.ts`, `inputs.ts` and preraid to P4 gear from upstream; no APLs,
  no P5. `tools/uitest/tests/settings/header.spec.ts` asserts the three stay out of the menu.
- Healers are out of scope in the parity, test-enrichment, bis-optimizer and caps PLANs; no workstream owns them.

## Decisions (the user, 2026-10-23)

- **Heal model: mana-limited casting.** Heals count in full on the dummies, overheal included; no incoming damage.
  Default rotations fill every GCD, and mana runs short over the default fight, so SP, crit, haste, Int,
  Spirit and MP5 all measure.
- **Stand-ins:** effects that need incoming damage (Chain Heal jumps, Earth Shield charges, Sacred Shield,
  Living Seed) get a stand-in trigger, as Prayer of Mending's 5 s proc does.
- **Order:** Resto Shaman first, then Holy Paladin and Resto Druid side by side.
- **Parity bar:** the project's usual ([server-parity](../guide/server-parity.md),
  [server-validation](../guide/server-validation.md)): spells declared against serverdata, each script rule from
  [ac] source a unit test, a short live simval stage per class under the RUNBOOK's standing authorizations.
- **Timing:** the HL waves run before Z18.
- **The EXPERIMENTAL alert** (`Tooltips.HEALING_SIM_DISCLAIMER`, an `alert()` on every healer page load) becomes
  an inline note on the page.
- **Healing Priest:** its APLs move to the fill-every-GCD model (its `spellCpm` caps leave haste and mana stats
  near zero in "Measure at my gear"), and it launches from Alpha.

## Out of scope

An incoming-damage model; healer BiS (optimizer, presetgen), shipped healer stat values (statweights) and
rankings; the raid sim's presets.

## Wave registry

All waves skip the BiS re-baseline: no DPS spec, preset or optimizer path moves.

| Wave | WIs |
|---|---|
| HL1 | HEAL-SHA (G) · HEAL-PRI (G) · HEAL-TEST (G) |
| HL2 | HEAL-PAL-1 (G) · HEAL-DRU-1 (G) |
| HL3 | HEAL-PAL-2 (G) · HEAL-DRU-2 (G) |

G changes only the class's own healer suites (and, for HEAL-TEST, the four healer suites' weights rows).

## Work items

**Every class item:**
- Spells, talents, glyphs, sets and relics as the server has them: ids and values from serverdata and the live
  item data, script rules from [ac] source with a unit test each. Server-side tweaks: none found in
  mod-spell-tweaks' SQL; re-check.
- A default APL (`ui/<spec>/apls/`, `autoRotation`) that fills every GCD with the class's usual priority.
- Presets per phase the server has (P1 to P5): upstream P1 to P4 gear checked against the server catalog
  (`assets/database/server_catalog.json`), P5 added; talents with glyphs; consumes.
- The default fight runs the P1 preset short of mana before its end. Report time to OOM per phase.
- Launch: `launched_sims.ts` to Launched, `header.spec.ts` updated.
- "Measure at my gear" on the page at each phase preset: every measured stat nonzero, or the item explains why
  (e.g. Spirit for a healer who never leaves the five-second rule).
- Last stage live: simval timing and spellmods checks for the new spells (`tools/simval`), the RUNBOOK's lock.
- Docs: [caster-mechanics](../guide/caster-mechanics.md) (druid, shaman, priest) or
  [class-mechanics](../guide/class-mechanics.md) (paladin), [damage-and-healing](../guide/damage-and-healing.md)
  for heal rules, [stat-caps](../guide/stat-caps.md) for the healer values; status and As built here.

### HEAL-SHA (G; stages: code, code, live)

- Tidal Force, Healing Way, the missing glyphs, T7 to T10 resto sets, the missing totems; Riptide and Earth Shield
  gated on their talents.
- Earth Shield charges from a stand-in "hit taken" trigger at the page's `earth_shield_p_p_m`. The trigger is a
  shared helper in a new `sim/core` file (healer stand-ins) for the later items.
- Chain Heal jumps across the dummies the way the server picks chain-heal targets ([ac] `Spell::SearchChainTargets`):
  check whether full-health units qualify, and follow it.
- APL: Riptide on cooldown, Earth Shield kept up, Chain Heal or Healing Wave filler with Tidal Waves, Mana Tide,
  Nature's Swiftness. Talent sets "Raid Healing" and "Tank Healing" kept.
- **Owns:** `sim/shaman/heals.go`, `chain_heal.go`, `sim/shaman/restoration/`, the shaman's heal sets, glyphs and
  totems, `ui/restoration_shaman/`, the new `sim/core` healer stand-ins file.

### HEAL-PRI (G; stages: code)

- `disc.apl.json` and `holy.apl.json` fill every GCD, no `spellCpm` caps; P5 presets; launch from Alpha.
- **Owns:** `sim/priest/healing/`, `ui/healing_priest/`.

### HEAL-TEST (G; stages: code)

- Golden StatWeights rows read HPS weights for healer specs (`sim/core/test_suite.go`, a test-enrichment carve-out).
- `statslopes_test.go`'s `request()` adds the dummies for every healer spec; `TestHealerIsMeasuredInHPS` covers
  each.
- The healer page's alert becomes an inline note (`ui/core/individual_sim_ui.ts`, `Tooltips.HEALING_SIM_DISCLAIMER`).
- **Owns:** those carve-outs, `sim/statslopes/statslopes_test.go`.

### HEAL-PAL-1 (G; stages: code, code, live)

- The Holy Paladin's heals and talents (the Missing list above, without glyphs, sets and librams).
- Sacred Shield's absorb from the stand-in trigger at its internal cooldown. Beacon of Light, Illumination and
  Infusion of Light per [ac] scripts.
- serverdata: rerun `gen_serverdata`; capture 20272 live.
- **Owns:** new heal files in `sim/paladin/`, `sim/paladin/holy/`, the paladin's serverdata allowlist and
  undeclared files.

### HEAL-DRU-1 (G; stages: code, code, live)

- The Resto Druid's heals, Tree of Life as an aura and form, and talents (the Missing list above, without glyphs,
  sets and idols).
- Living Seed blooms from the stand-in trigger. `SpellFlagNaturesGrace` and `SpellFlagOmenTrigger` on the heals.
- serverdata: rerun `gen_serverdata`; capture 48503, 48504, 48540, 48542, 48543 live.
- **Owns:** new heal files in `sim/druid/`, `sim/druid/restoration/`, the druid's serverdata allowlist and
  undeclared files.

HL2's two items both regenerate `spells_auto_gen.go`: at integration, the second merge reruns `gen_serverdata`
on the merged tree instead of resolving the conflict by hand.

### HEAL-PAL-2 (G; stages: code, live)

- Holy glyphs, T7 to T10 holy sets, healing librams, the APL (Beacon kept up, Sacred Shield, Holy Shock on
  cooldown, Holy Light or Flash of Light filler, Divine Illumination, Divine Plea), presets, launch.
- **Owns:** the paladin's holy glyphs, sets and librams, `ui/holy_paladin/`.

### HEAL-DRU-2 (G; stages: code, live)

- Resto glyphs, T7 to T10 resto sets, healing idols, the APL (Lifebloom stack, Rejuvenation and Wild Growth kept
  rolling, Swiftmend, Nourish filler), presets, launch.
- **Owns:** the druid's resto glyphs, sets and idols, `ui/restoration_druid/`.
