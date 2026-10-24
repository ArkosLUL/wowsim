# Healer sims: Holy Paladin, Resto Druid and Resto Shaman pages that heal

Status: approved by the user (2026-10-23); HL1 landed; HL2 running.

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
- **Default fight:** 300 s on healer pages, so mana stats measure where 180 s ends before the mana does (Disc P4
  and P5).

## Out of scope

An incoming-damage model; healer BiS (optimizer, presetgen), shipped healer stat values (statweights) and
rankings; the raid sim's presets.

## Wave registry

All waves skip the BiS re-baseline: no DPS spec, preset or optimizer path moves.

| Wave | WIs |
|---|---|
| HL1 | HEAL-SHA (G) · HEAL-PRI (G) · HEAL-TEST (G) |
| HL2 | HEAL-FIGHT · HEAL-PAL-1 (G) · HEAL-DRU-1 (G) |
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

#### As built

Stage 1, spells (uncommitted):
- `core.HitTakenStandIn` (`sim/core/healer_stand_ins.go`): a hit every 60 / rate s from `Start`, for the later items.
  Earth Shield's charges ride it at `earth_shield_p_p_m`, 3.5 s apart per target.
- Chain Heal follows the server: a full-health unit doesn't qualify, so the dummies count as missing 1 health
  (`standInDeficit`) to give jumps. Also fixed: the Riptide check is the first target's only (+25% on every heal of
  the cast), and every target's crit is rolled before any heal lands.
- Tidal Force (no major cooldown: the APL casts it), Healing Way, Riptide and Earth Shield behind their talents,
  Glyphs of Healing Wave (55533), Riptide, Mana Tide and Water Mastery, T7 to T10 resto sets, and the Misery,
  Relentless, Wrathful, Calming Tides and Surging Sea totems. Glyph of Water Shield does nothing (orbs need hits).
  Glyph of Healing Stream Totem isn't done: the totem's script and 52042 both read the glyph, unchecked.
- Needs a live capture (stage 3): 55533 and 70809 (`tmp/spellids-missing.txt`), then `gen_serverdata`. 70809's
  tick count, damage class and flags are from Spell.dbc.
- Also touched: `implemented_talents.json` (regenerated), `sim/optimizer/residuals.go` (3 totem ids),
  `sim/core/buffs.go` (`ManaTideTotemAuraRestoring`), `sim/core/healing_done.go` (`HealingDoneMultiplier`),
  `spells_auto_gen.go` and friends (regenerated, additions only).
- Goldens: P1 gear has T7 2/4-piece, so Resto's stats gain 10 MP5 and Chain Heal 5% (Healing Grace suite HPS
  +5.7%); every shaman suite gains `AllItems` rows for the new sets and totems.

Stage 2, page (uncommitted):
- Default APL `ui/restoration_shaman/apls/default.apl.json` as `autoRotation` (rule in
  [caster-mechanics](../guide/caster-mechanics.md)); `earthShieldPPM` 30, totems in `DefaultOptions`; P5 gear
  (Val'anyr, Surging Sea); Raid and Tank Healing talents and glyphs kept; launched.
- `sim/core/raid.go`: a target dummy gets no character effects or health bar even in a slot the page sent as a
  blank player (the page pads parties to 5). Without it the page healed ~184 HPS. No golden moves.
- Tests (`default_apl_test.go`): the page's request heals with every APL spell, runs P1 dry inside 180 s, and
  every preset item is in the server catalog by its phase's tier; new golden row `Rotation-Default-Average`.
- Page request, 180 s: time to OOM preraid 129 s, P1 153, P2 120, P3 176, P4 and P5 none (HPS 6975 to 22043).
- "Measure at my gear" on the page, HPS a point (Int / SP / crit / haste / MP5): P1 4.07 / 2.32 / 2.30 / 0.64 /
  3.43; P3 4.62 / 5.37 / 3.79 / 3.36 / 2.65; P5 1.37 / 3.22 / 1.97 / 3.10 / 0.22. Spirit ≤ 0.19, exactly 0 at P4.

Stage 3, live (uncommitted; server lock held 17:37 to 17:56 UTC, no rebuild):
- Spell capture: 55533 and 70809 captured (`TestSpelldumpCapture`), added to `spelldump.jsonl`; `gen_serverdata`
  rerun (additions only); `spellids` reports nothing missing.
- `talent_paths.json` rewritten (`TestTalentPathsFile -update`: ShamanRestoration gains tidalForce, healingWay,
  riptide; earthShield stops being a filler) and `spellmod_spells.json` regenerated (`tools/acore/spellmods`:
  Healing Way, and 55166/55198 under several shaman talents). Stage 1 left `TestTalentPathsFile` and
  `TestTalentPathsAreLegal` failing; both pass now.
- Talent sweep, ShamanRestoration (`SIMVAL_TALENT_ONLY`), trimmed into `talents.jsonl`: every step agrees or
  moves no field. Spell-mod sweep, ShamanElemental and ShamanRestoration (their lines replaced in
  `spellmods.jsonl`): Restoration 32 agree, 13 move nothing, 14 not compared, none disagree (Healing Way ×1.2498
  on both sides); Elemental 40 / 10 / 26, none disagree. `TestTalentSweepRecords`, `TestSpellModRecords` pass.
- Timing: a private copy of the e2e harness (not committed) took `.simval timing` of Riptide, Chain Heal, Healing
  Wave, Lesser Healing Wave, Earth Shield, Tidal Force, Mana Tide, Nature's Swiftness, Healing Stream,
  Ancestral Awakening, Earthliving, Tidal Waves, Chained Heal and Rapid Currents: bare, talented with Glyphs of
  Riptide, Earth Shield and Healing Wave, under Tidal Waves, T8 and T10 bonus auras, and Bloodlust (49 records),
  replayed through `replayTiming`. Agree: every cast time, GCD, cooldown, cost, aura duration and tick count
  of the new spells; Glyph of Riptide 21 s and 7 ticks; T8 Riptide 5 s and Chain Heal 2.3 s; Rapid Currents
  ×1/1.2 on cast time and GCD; Chained Heal 9 s, 3 ticks.
- Fixed from it: Tidal Waves' buff never expired (server 15 s); the shaman got Mana Tide twice, its own and
  core's external one its talent added to its own party (`core.ManaTideCaster`, `sim/core/buffs.go`). Tests in
  `mana_tide_test.go`.
- Left: Healing Stream's 1 s GCD (allowlisted, Call of the Elements); Earthliving's hot sits on 51994 (12 s, 4
  ticks, as the server's 52000).
- Page request after the fixes, 180 s (time to OOM, HPS): preraid 111 s 6496, P1 130 s 8842, P2 102 s 11183, P3
  147 s 13378, P4 none 20921, P5 166 s 21205. HPS a point, `statslopes.Measure` at the page's request, 3000
  iterations (Int / SP / crit / haste / MP5): P1 4.12 / 2.19 / 2.26 / 0.52 / 4.27; P3 6.77 / 4.73 / 4.15 / 2.83 /
  5.55; P5 6.37 / 3.17 / 4.14 / 1.98 / 5.77. Spirit ≤ 0.24, 0 at P4. These replace stage 2's.

Review (uncommitted):
- T10 4-piece's 70809 now adds what's left of the running hot and lands on the shaman's next 400 ms step
  (`Unit::CastDelayedSpellWithPeriodicAmount`; `core.DelayedPeriodicApplier`).
- Glyph of Healing Wave's 55533 drops the healing done percents (`SPELL_ATTR6_IGNORE_HEALTH_MODIFIERS`);
  `Unit.HealingDoneMultiplier` removed. Neither moves a golden.
- Page request after the fixes, same method: P4 none, HPS 21460; P5 166 s, 21619 (mana unchanged). P5 HPS a point
  (Int / SP / crit / haste / MP5): 6.50 / 3.18 / 4.46 / 2.77 / 5.81. These replace stage 3's P5.

Cross-review (uncommitted):
- `TestRestoration` runs the page's APL with `IsHealer` and Earth Shield at 30 a minute, so its Settings,
  AllItems, Average and StatWeights rows heal (they were 0 HPS); only that suite's goldens move.
- The real page's request matches the numbers above; at P4 ~3 s out of mana on average (avg time to OOM 186 s).

### HEAL-PRI (G; stages: code)

- `disc.apl.json` and `holy.apl.json` fill every GCD, no `spellCpm` caps; P5 presets; launch from Alpha.
- **Owns:** `sim/priest/healing/`, `ui/healing_priest/`.

#### As built

- APLs: no `spellCpm` caps. Disc: Power Word: Shield on 2 targets, Prayer of Mending, Penance, Greater Heal
  filler. Holy: Circle of Healing, Prayer of Mending, Renew on 3 targets, Greater Heal filler. A Flash Heal
  filler spent no more mana, so Greater Heal stayed.
- P5 presets (`p5_disc`, `p5_holy`): ICC heroic cloth plus the Ruby Sanctum cloak, ring and trinket, gems
  matched to sockets. Both trees share one list. `p4_holy` was a copy of `p3_holy` (ToC gear); it now holds the
  P4 list. The page is Launched; `header.spec.ts` needed no change (Alpha was already in the menu).
- Measured on the merged tree (cross-review) with the page's own `/statSlopesAsync` request (padded parties,
  9 dummies, 180 s, 3000 iterations, seed 424242). Until HEAL-SHA's `raid.go` rule the page healed ~350 HPS
  (its dummies were dead), so the numbers first reported here came from another request. Time to OOM, s
  out of mana in brackets, HPS:

  | | preraid | P1 | P2 | P3 | P4 | P5 |
  |---|---|---|---|---|---|---|
  | Disc | 139 (20) 5945 | 150 (14) 7072 | 155 (11) 7973 | 198 (0) 9051 | 325 (0) 10614 | 281 (0) 11358 |
  | Holy | 84 (62) 5540 | 96 (49) 6605 | 88 (53) 7433 | 63 (68) 8313 | 154 (10) 12491 | 123 (29) 12476 |

- "Measure at my gear": every stat is nonzero except:
  - Disc P4 and P5, Spirit and MP5 (0): the fight ends before mana does (480 s at P4: Spirit 1.45, MP5 3.27).
  - Spell haste's gain is about 0 or below on disc P1 and P2 (−0.07, −0.18) and holy at every phase but P3
    (−0.21 to −0.29): mana-bound, faster casts spend more of it on the Greater Heal filler. Unbound (holy P1
    over 60 s) haste gains 1.05.
- `page_request_test.go` (cross-review): the page's request heals with every APL spell and runs dry inside
  180 s at P1, both trees.

### HEAL-TEST (G; stages: code)

- Golden StatWeights rows read HPS weights for healer specs (`sim/core/test_suite.go`, a test-enrichment carve-out).
- `statslopes_test.go`'s `request()` adds the dummies for every healer spec; `TestHealerIsMeasuredInHPS` covers
  each.
- The healer page's alert becomes an inline note (`ui/core/individual_sim_ui.ts`, `Tooltips.HEALING_SIM_DISCLAIMER`).
- **Owns:** those carve-outs, `sim/statslopes/statslopes_test.go`.

#### As built

- `TestStatWeights` reads `Hps` weights when `core.IsHealerSpec` (new, `test_suite.go`); `HealerStatsToWeigh` is the
  six healer stats. `FullCharacterTestSuiteGenerator` (`test_generators.go`, outside the Owns line) defaults a
  healer suite's `StatsToWeigh` to them, SP as reference, so the three suites that had none gain a row.
- `StatWeightsRequest` has no target dummies field (a proto change; not made). Cross-review: `TestStatWeights`
  adds one for a healer through `calcStatWeight` (`sim/core/statweight.go`), as `IsHealer` does for the other
  rows; `CalcStatWeight` and the UI are unchanged.
- Rows: Disc, Holy priest and (cross-review: the suite runs the page's APL) Resto Shaman nonzero; Holy Paladin
  and Resto Druid all zero until they cast heals.
- `statslopes_test.go`: player builders for the three other healers (P1 gear; Resto Shaman on the page's APL,
  the others autocast-only), `request()` adds a dummy for every healer spec, `TestHealerIsMeasuredInHPS` runs
  all four (HPS above zero asserted for the priest and the shaman).
- The alert is gone: healer pages add a warning-zone note (`Tooltips.HEALING_SIM_DISCLAIMER`, reworded to say
  what the model is).
- Goldens: only the five healer suites' StatWeights rows moved; dps, hps, tps, dtps and abilities unchanged in all 37.

### HEAL-FIGHT (stages: code)

- Healer pages default to a 300 s fight, one rule for every `isHealingSpec` page (`ui/core/individual_sim_ui.ts`,
  `Encounter.applyDefaults`); other pages keep 180 s.
- The page-request tests (`sim/shaman/restoration/default_apl_test.go`, `sim/priest/healing/page_request_test.go`)
  send 300 s and still assert P1 runs dry before the end.
- Re-measure Disc, Holy and Resto Shaman on the page's request at 300 s, each phase preset: time to OOM and
  "Measure at my gear", every stat nonzero or explained. These replace HEAL-SHA's and HEAL-PRI's numbers.
- Golden-neutral: report any golden that moves.
- **Owns:** the encounter-default lines of `ui/core/individual_sim_ui.ts` and `ui/core/encounter.ts`, those two
  tests, the 180 s fight facts in `docs/guide/caster-mechanics.md`.

#### As built

- `Encounter.applyDefaults(eventID, defaultTarget?, healingSpec)`: 300 s for a healer, else 180; the individual page
  passes `isHealingSpec`, the raid sim nothing (180). Settings a browser saved earlier, and links, carry their own
  encounter and keep their duration; only "Restore Defaults" moves them to 300 s.
- Tests: both `pageRequest`s send 300 s and the OOM checks read it. `defaultAPLGoldenRequest` keeps 180 s, so
  `Rotation-Default-Average` doesn't move. The priest's `pageRequest(tree, phase)` now matches what the page sends
  (Draenei, 200 ms reaction, no clip delay, Engineering and Jewelcrafting, Totem of Wrath): at 180 s it reproduces
  HEAL-PRI's numbers (Undead ran ~1% lower). The shaman's differs only in fields its heals don't read (reaction time,
  clip delay, Jewelcrafting, minor glyphs): within 0.1%.
- Measured with `pageRequest` per tree and phase, 300 s, 3000 iterations, seed 424242; slopes by
  `statslopes.Measure`. These replace HEAL-SHA's and HEAL-PRI's numbers. Time to OOM, s out of mana in brackets, HPS
  (past 300 s is the sim's estimate):

  | | preraid | P1 | P2 | P3 | P4 | P5 |
  |---|---|---|---|---|---|---|
  | Disc | 139 (79) 5114 | 150 (67) 6155 | 155 (65) 6912 | 189 (44) 7993 | 373 (0) 10444 | 328 (0) 11157 |
  | Holy | 84 (140) 4523 | 96 (120) 5503 | 88 (125) 6282 | 61 (145) 7217 | 154 (57) 11055 | 123 (90) 10737 |
  | Resto | 111 (118) 5273 | 130 (91) 7265 | 102 (125) 8537 | 147 (84) 10550 | 181 (65) 16827 | 165 (83) 16230 |

- "Measure at my gear", HPS a point, gain/loss:

  | | Int | Spirit | SP | MP5 | Crit | Haste |
  |---|---|---|---|---|---|---|
  | Disc preraid | 2.20/2.08 | 0.85/0.75 | 1.06/1.07 | 2.18/2.26 | 0.76/0.77 | 0.04/−0.04 |
  | Disc P1 | 2.48/2.36 | 0.96/0.96 | 1.13/1.13 | 2.46/2.50 | 0.90/0.92 | −0.06/0.10 |
  | Disc P2 | 2.88/2.75 | 1.04/1.08 | 1.17/1.17 | 2.80/2.72 | 1.00/1.00 | −0.10/0.15 |
  | Disc P3 | 3.07/3.08 | 1.21/1.22 | 1.27/1.27 | 3.09/2.97 | 1.14/1.14 | 0.18/−0.05 |
  | Disc P4 | 0.80/0.61 | 0.21/−0.00 | 1.50/1.50 | 0.31/0.25 | 1.41/1.42 | 1.20/2.26 |
  | Disc P5 | 0.54/2.43 | 0.02/0.53 | 1.56/1.56 | 0.07/2.26 | 1.51/1.52 | 0.45/2.00 |
  | Holy preraid | 2.17/2.34 | 2.04/2.35 | 1.09/1.10 | 2.51/3.44 | 0.33/0.34 | −0.14/−0.07 |
  | Holy P1 | 3.60/2.66 | 3.11/2.51 | 1.25/1.25 | 4.59/2.78 | 0.37/0.37 | −0.13/−0.23 |
  | Holy P2 | 1.90/2.81 | 1.82/2.38 | 1.27/1.26 | 4.17/5.42 | 0.63/0.63 | −0.14/0.01 |
  | Holy P3 | 2.16/3.16 | 2.46/3.16 | 1.35/1.35 | 4.00/4.64 | 0.67/0.69 | 0.08/−0.04 |
  | Holy P4 | 2.96/2.60 | 3.13/3.41 | 1.69/1.69 | 4.35/5.40 | 0.81/0.80 | −0.09/−0.05 |
  | Holy P5 | 2.26/1.73 | 1.64/2.04 | 1.56/1.55 | 3.47/5.52 | 0.70/0.70 | −0.10/−0.30 |
  | Resto preraid | 2.89/2.44 | 0.33/0.14 | 1.47/1.47 | 4.42/4.41 | 1.43/1.33 | 0.05/0.20 |
  | Resto P1 | 3.14/3.38 | 0.12/0.19 | 1.82/1.81 | 4.98/4.70 | 1.97/1.94 | 0.31/0.54 |
  | Resto P2 | 4.57/4.29 | 0.44/0.23 | 2.51/2.74 | 7.29/5.86 | 2.74/2.61 | 0.97/0.50 |
  | Resto P3 | 4.90/4.36 | −0.01/0.17 | 3.36/3.34 | 5.95/6.67 | 3.24/2.90 | 1.59/1.51 |
  | Resto P4 | 6.66/7.06 | −0.02/0.13 | 2.92/3.18 | 9.65/9.67 | 4.78/4.58 | 0.72/3.60 |
  | Resto P5 | 6.52/7.76 | 0.36/0.44 | 2.54/2.60 | 10.42/11.19 | 4.12/4.07 | 1.62/1.92 |

  Every stat is nonzero except:
  - Disc P4 and P5 still don't run dry: Spirit and MP5 gain 0.21/0.31 (P4) and 0.02/0.07 (P5), and read only on the
    loss side (P5 0.53, 2.26). 300 s meets the decision's aim there only halfway (480 s at P4: Spirit 1.45, MP5 3.27).
  - Haste about 0 or below while mana-bound: holy at every phase, disc preraid to P3.
  - Resto Spirit about 0 (−0.02 to 0.44): the five-second rule.
- HPS falls where the longer fight leaves more time out of mana (Resto P4 21460 → 16827).
- Review: at 300 s the Resto APL without its Healing Wave line out-heals the default by 10–27% at every phase and
  still runs dry (preraid 6553, P1 9234 at ~255 s, P2 9407, P3 12099, P4 20662, P5 20444 HPS); at 180 s P1 it's +14%,
  not the ~6% the guide had. The APL is HEAL-SHA's, left as is.

### HEAL-PAL-1 (G; stages: code, code, live)

- The Holy Paladin's heals and talents (the Missing list above, without glyphs, sets and librams).
- Sacred Shield's absorb from the stand-in trigger at its internal cooldown. Beacon of Light, Illumination and
  Infusion of Light per [ac] scripts.
- serverdata: rerun `gen_serverdata`; capture 20272 live.
- **Owns:** new heal files in `sim/paladin/`, `sim/paladin/holy/`, the paladin's serverdata allowlist and
  undeclared files.

#### As built

Stage 1, spells (uncommitted):
- Holy Light, Flash of Light, Holy Shock, Beacon of Light (copies 53652-4, Lay on Hands too), Sacred Shield (58597 on
  the stand-in at its cooldown), Divine Favor, Divine Illumination, Seals of Wisdom and Light with 54158, in
  `sim/paladin/{heals,holy_shock,beacon_of_light,sacred_shield,sow_sol}.go`; rules in
  [class-mechanics](../guide/class-mechanics.md#paladin) and [damage-and-healing](../guide/damage-and-healing.md#healing).
  Divine Guardian's Sacred Shield mods taken too; tests `sim/paladin/holy/heals_test.go`.
- Also touched: `paladin.go`, `lay_on_hands.go`; Holy gets a non-swinging main hand and the first dummy as target.
- Generated: `gen_serverdata` (additions only), `implemented_talents.json`, `spellmod_spells.json`,
  `talent_paths.json`. `tools/simval`'s `TestTalentSweepRecords` and `TestSpellModRecords` fail until stage 3
  re-records the paladin sweeps.
- `TestHoly` runs an APL casting every heal (beacon on self): Average HPS 0 → 6650; StatWeights nonzero.
- Left for stage 2: Healing Light, Sanctified Light, Holy Power, Light's Grace, Infusion of Light (66922 on a Sacred
  Shield target), Illumination (needs 20272 captured), Judgements of the Pure (+5% a point on 54158 too), Holy
  Guidance, Improved Flash of Light, Purifying Power.

Stage 2, talents (uncommitted):
- Healing Light, Sanctified Light, Holy Power, Holy Guidance, Enlightened Judgements, Light's Grace, Infusion of Light
  (with 66922), Illumination, Judgements of the Pure (haste, and its damage mods on the seal and judgement spells in
  `sor.go`, `sov.go`, `soc.go`, `sow_sol.go`) in `sim/paladin/holy_talents.go`; rules in
  [class-mechanics](../guide/class-mechanics.md#paladin) and [damage-and-healing](../guide/damage-and-healing.md#healing).
  Tests `sim/paladin/holy/talents_test.go`; stage 1's crit tests now zero the talents' crit.
- Not done: Spiritual Focus (no pushback model); Purifying Power (no golden build takes it; it needs an `untakenTalents`
  entry in `sim/talent_coverage_test.go`, outside this item). "Improved Flash of Light" isn't a 3.3.5 talent.
- Needs a live capture (stage 3): 20272 and 66922; their data here is from Spell.dbc.
- Generated: `gen_serverdata` (additions only), `implemented_talents.json` (+12 holy talents), `spellmod_spells.json`,
  `talent_paths.json` (holy steps on PaladinProtection2 and PaladinRetribution).
- `TestHoly` Average HPS 6650 → 9975; spell power +327 (Holy Guidance), 4% hit; StatWeights Int 4.04, SP 2.31,
  MP5 2.31, crit 1.34, haste 0.63.

Stage 3, live (uncommitted; server lock held 21:19 to 21:37 UTC, no rebuild):
- Spell capture: 20272 and 66922 (`TestSpelldumpCapture`) added to `spelldump.jsonl`; `gen_serverdata` rerun
  (additions only; 20272's energize threat moves only `TestHoly`'s TPS); `spellids` reports nothing missing.
- `costaura_spells.json`, stale since stage 1 (Divine Illumination, aura 72), rewritten (`TestCostAuraSpellsFile
  -update`); `spellmod_spells.json` and `talent_paths.json` unchanged.
- Sweeps of PaladinProtection, PaladinProtection2 and PaladinRetribution, their lines replaced in `talents.jsonl`
  and `spellmods.jsonl`, added to `costauras.jsonl`; none disagree. Talent: 17 agree, 48 move no field. Spell-mod:
  54 agree, 53 move nothing, 17 not compared. Cost-aura: Divine Illumination's -50% agrees on 22 spells.
  - 48821 and 48823 aren't compared: the replay finds no talent teaching them (first ranks 25914, 25912). Taught
    Holy Shock by hand, Healing Light ×1.12, Divine Favor's and Sanctified Light's crit agree.
- Timing: a private copy of the e2e harness (not committed) took `.simval timing` of the 21 spells stages 1 and 2
  added: bare; under Benediction, Divine Guardian, Light's Grace, Infusion of Light and Judgements of the Pure;
  then each of 31834, 54149, 31842, 54153 and Bloodlust on top (141 records). Replayed through `replayTiming`
  (plus Light's Grace's and Infusion's `ModifyCast` cut): every cast time, GCD, cooldown, cost, aura duration and
  tick count agrees.
  - Probing 54153 puts 15% haste on and off, leaving the server's float cast speed just under 1, so every later
    probe reads 1 ms short: probe it last.
- Fixed: Divine Favor's cooldown started at cast; the server's starts as its buff goes. Test
  `TestDivineFavorCooldownStartsWhenUsed`; `TestHoly` Average HPS 9975 → 9974.

Review (uncommitted):
- Judgement of Command (20467) now takes Judgements of the Pure's +5% a point: its flags[1] 0x200 is in the
  talent's `SPELLMOD_DAMAGE` mask (0x20400e00). No golden build has both; JoC is unprobed in the sweep.

### HEAL-DRU-1 (G; stages: code, code, live)

- The Resto Druid's heals, Tree of Life as an aura and form, and talents (the Missing list above, without glyphs,
  sets and idols).
- Living Seed blooms from the stand-in trigger. `SpellFlagNaturesGrace` and `SpellFlagOmenTrigger` on the heals.
- serverdata: rerun `gen_serverdata`; capture 48503, 48504, 48540, 48542, 48543 live.
- **Owns:** new heal files in `sim/druid/`, `sim/druid/restoration/`, the druid's serverdata allowlist and
  undeclared files.

HL2's class items both regenerate `spells_auto_gen.go`, `implemented_talents.json`, `talent_paths.json` and
`spellmod_spells.json`, and add to `spelldump.jsonl` and the sweep records: at integration, the second merge
keeps both items' lines and reruns the generators on the merged tree instead of resolving them by hand.

#### As built

Stage 1, spells (uncommitted):
- Healing Touch, Regrowth, Rejuvenation, Lifebloom with its bloom (33778) and mana (64372), Nourish, Wild Growth,
  Swiftmend and Tree of Life (form, raid aura 34123, passive 5420), registered for Restoration only
  (`RegisterRestorationSpells`); rules in [caster-mechanics](../guide/caster-mechanics.md#druid) and
  [damage-and-healing](../guide/damage-and-healing.md#healing). The druid heals its first target dummy.
- Also taken now on the heals: the already implemented Genesis, Moonglow, Nature's Majesty, Nature's Splendor,
  Naturalist and Natural Shapeshifter (the spell-mod list's mods on them). Left for stage 2: the Missing list's
  talents, Master Shapeshifter's tree form, Gift of the Earthmother, Subtlety.
- Core: `Dot.HotTickHealing` (`sim/core/healing_done.go`) for Swiftmend.
- Regenerated (additions only): `gen_serverdata`, `implemented_talents.json` (swiftmend, treeOfLife,
  wildGrowth), `talent_paths.json`, `spellmod_spells.json`. Nothing missing from the capture.
- `TestRestoration` runs an in-test APL casting every heal, with `IsHealer`: Average HPS 0 to 3862, HPS weights
  nonzero. Tests: `lifebloom_test.go`, `heals_test.go`, `tree_of_life_test.go`.

Stage 2, talents (uncommitted):
- The Missing list's talents plus Master Shapeshifter's tree form, Gift of the Earthmother and Subtlety (on
  Innervate too); rules in [caster-mechanics](../guide/caster-mechanics.md#druid). Living Seed and Revitalize in
  `sim/druid/resto_talents.go`; tests in `talents_test.go`. Nature's Focus does nothing.
- Hots no longer read the spell's `DamageMultiplier` (the direct heal's). Tree of Life now goes on in
  `Druid.Reset`, not its OnReset, so Master Shapeshifter survives the spells' reset.
- Needs a live capture (stage 3): 48503, 48504, 48540 to 48543 (`tmp/spellids-missing.txt`; 48541 beyond the
  Context's list), then `gen_serverdata`; after it `rollsOmenOffFamily` decides Omen on 48503 and 48504.
- New golden row `Talents-LivingSeed-Average` (`LivingSeedTalents`: Improved Tree of Life's points in Living Seed)
  and its `serverDataPresets` entry (`sim/serverdata_test.go`). Average HPS 3862 to 6079.
- Regenerated: `gen_serverdata` (Living Seed's procs), `implemented_talents.json`, `talent_paths.json`,
  `spellmod_spells.json`.

Stage 3, live (uncommitted; server lock held 21:37 to 21:52 UTC, no rebuild):
- Captured 48503, 48504, 48540 to 48543 (`TestSpelldumpCapture`), added to `spelldump.jsonl`; `gen_serverdata`
  rerun (additions only: 48504's proc row, 48503's threat row); `spellids` reports only 26545 and 53258;
  `spellmod_spells.json` unchanged.
- With its data, 48503 rolls Omen of Clarity ([caster-mechanics](../guide/caster-mechanics.md#druid)):
  `Talents-LivingSeed-Average` HPS 6100 to 6236, all from it (checked by turning it off).
- Talent sweep, DruidFeralBear and DruidRestoration (`SIMVAL_TALENT_ONLY`), trimmed into `talents.jsonl`: every
  step agrees or moves no field. Improved Tree of Life moves nothing: no path takes the form.
- Spell-mod sweep, the four druid paths, their lines replaced in `spellmods.jsonl`. The heals compare as
  `restoration_druid` (`timingExtraSpecs`, `tools/simval/timing.go`). Druid: 74 agree, 73 move nothing, 42 not
  compared, 1 disagrees (Berserk on Mangle, already dropped). Agree: Moonglow and Tranquil Spirit cost, Nature's
  Majesty and Nature's Bounty crit, Nature's Splendor durations and ticks, Naturalist and Nature's Swiftness cast
  time, Gift of Nature and Genesis on Healing Touch and Regrowth, Empowered Touch and Rejuvenation coefficients,
  Gift of the Earthmother's Lifebloom GCD. The scripted hots' and Nourish's amounts aren't compared (shaped).
- Timing: a private copy of the e2e harness (not committed) took 123 `.simval timing` records of Healing Touch,
  Regrowth, Rejuvenation, Lifebloom, its bloom, Nourish, Wild Growth, Swiftmend, Tree of Life, 48504 and 48503:
  bare, under Naturalist, Moonglow, Nature's Splendor, Tranquil Spirit, Gift of the Earthmother, Natural
  Shapeshifter and 5420 each alone, all together, with Bloodlust on top, and under Clearcasting. Replayed through
  `replayTiming`: every cast time, GCD, cooldown, cost, duration and tick count agrees (e.g. Lifebloom's GCD
  1227 ms under Gift of the Earthmother; cost mods add, Healing Touch 933; Clearcasting frees every heal, not
  Tree of Life). Nothing needed fixing.
- Not checked live: Tree of Life's form effects, Living Seed's and Revitalize's procs, the hots' amounts (unit
  tests cover them).

Review fixes (uncommitted):
- 34123 reaches the whole raid (effect 65), not the party (`TestTreeOfLifeHealsTheRaidMore`).
- Swiftmend eats another druid's Rejuvenation or Regrowth when the druid has none
  (`TestSwiftmendFallsBackToAnotherDruidsHot`).
- Goldens unchanged by both: the suite's one dummy shares the druid's party, and no other druid plays.

### HEAL-PAL-2 (G; stages: code, live)

- Holy glyphs, T7 to T10 holy sets, healing librams, the APL (Beacon kept up, Sacred Shield, Holy Shock on
  cooldown, Holy Light or Flash of Light filler, Divine Illumination, Divine Plea), presets, launch.
- **Owns:** the paladin's holy glyphs, sets and librams, `ui/holy_paladin/`.

### HEAL-DRU-2 (G; stages: code, live)

- Resto glyphs, T7 to T10 resto sets, healing idols, the APL (Lifebloom stack, Rejuvenation and Wild Growth kept
  rolling, Swiftmend, Nourish filler), presets, launch.
- **Owns:** the druid's resto glyphs, sets and idols, `ui/restoration_druid/`.
