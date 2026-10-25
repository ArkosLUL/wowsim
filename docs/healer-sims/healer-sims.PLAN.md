# Healer sims: Holy Paladin, Resto Druid and Resto Shaman pages that heal

Status: approved by the user (2026-10-23); HL1 to HL3 landed; all four healer pages launched. HL4 (the user,
2026-10-25): effective healing and the haste caps.

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

- **Heal model (HL1 to HL3; HL4 replaces it): mana-limited casting.** Heals count in full on the dummies, overheal
  included; no incoming damage. Default rotations fill every GCD, and mana runs short over the default fight, so
  SP, crit, haste, Int, Spirit and MP5 all measure.
- **Stand-ins (HL1 to HL3; HL4 replaces them with the dummies' hits):** effects that need incoming damage (Chain
  Heal jumps, Earth Shield charges, Sacred Shield, Living Seed) get a stand-in trigger, as Prayer of Mending's 5 s
  proc does.
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
- **HL3 also fixes (2026-10-24)** the HL2 cross-review's open points, though DPS goldens may move: Innervate's
  inverted condition, no-damage hits proccing rows without NO_DMG_HEAL, Talisman of Troll Divinity's stacks, and the
  Resto Shaman default APL loses its Healing Wave line.
- **Val'anyr (2026-10-24):** its shields keep counting in full as healing, though the dummies never consume them.

**HL4 (the user, 2026-10-25).** The healer values disagreed with the class guides (Resto Druid reforged Spirit
into crit, Resto Shaman ranked crit over haste) because overheal counted. So:
- **Effective healing:** the dummies take damage, and a heal counts only up to the missing health; the rest is
  overheal. Raw HPS stays shown.
- **Incoming damage:** default per spec and phase, about what the phase's preset heals per second when every heal
  lands in full, so it can just about keep up; a page setting changes it.
- **Shields** keep counting in full as they go on (the Val'anyr call stands); the dummies' damage ignores them.
- **Rotations** heal the most hurt dummy and wait when nobody is hurt; HoTs and shields may go out ahead of damage
  where the class's usual play does.
- **Haste caps:** Wrath of Air on by default on the Resto Druid and Holy Paladin pages, as on Disc and Resto
  Shaman; the Disc haste cap also gives the number right after Power Word: Shield (Borrowed Time).

## Out of scope

Healer BiS (optimizer, presetgen), shipped healer stat values (statweights) and
rankings; the raid sim's presets.

## Wave registry

HL1 and HL2 skip the BiS re-baseline: no DPS spec, preset or optimizer path moves. HL3 runs it if HEAL-CORE or
the Innervate fix moves a DPS suite. HL4 runs it only if a DPS suite moves.

| Wave | WIs |
|---|---|
| HL1 | HEAL-SHA (G) · HEAL-PRI (G) · HEAL-TEST (G) |
| HL2 | HEAL-FIGHT · HEAL-PAL-1 (G) · HEAL-DRU-1 (G) |
| HL3 | HEAL-CORE · HEAL-PAL-2 (G) · HEAL-DRU-2 (G) |
| HL4 | HEAL-CAPS · HEAL-EFFECTIVE (G, FS) |

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
- HL2 cross-review: no test guarded the 300 / 180 s split (the page-request tests read the request's own fight).
  `tools/uitest/tests/settings/default_fight.spec.ts`: 300 s on the four healer pages, 180 s on every other page
  and the raid sim, a saved fight kept until Restore Defaults.

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
  - 48821 and 48823 are taught through 48825's talent (`spellModCastBy`, HL2 cross-review): Healing Light ×1.12,
    Divine Favor's and Sanctified Light's crit agree; Benediction moves nothing.
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
- HL2 cross-review: a judgement at a friend (Holy's default target) crashed the sim on its nil enemy aura; it now
  isn't cast (`canJudgement`, `TestNoJudgementOnAFriend`). Goldens unchanged.

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

HL2 cross-review (uncommitted):
- Lifebloom's ticks and bloom multiply the flat healing taken by the stacks (`scaleFlatTaken`); Swiftmend's ticks
  take Strength of Wrynn; Faerie Fire at a friend (the druid's default target) crashed the sim and now isn't cast.
  Tests: `TestLifebloomFlatHealingTakenCountsTheStacks`, `TestSwiftmendTakesStrengthOfWrynn`,
  `TestNoFaerieFireOnAFriend`, `TestRevitalizeOnRejuvenation`; Clearcasting, Living Seed's Regrowth and Swiftmend
  plants checked. Goldens unchanged: Talisman of Troll Divinity never stacks its 60518, and the suite's buffs lack
  Strength of Wrynn.
- Open: Innervate never casts (`registerInnervateCD`'s inverted `ExtraCastCondition`, from upstream; fixing it moves
  Balance and Feral pages with an Innervate target); hot landings proc trinkets whose rows lack NO_DMG_HEAL (core
  `ApplyProcTriggerCallback`); Talisman of Troll Divinity never stacks (`sim/common`).

**HL3 class items, both:**
- Judgements, Faerie Fire and other damage on the boss (`Target`), never the friendly current target.
- The page guard test is built on the page's padded 9-dummy request at 300 s, like the priest's and shaman's
  `pageRequest`. "Measure at my gear" is measured from it, and the spec's row in `TestHealerIsMeasuredInHPS`
  (`sim/statslopes`) gets `heals: true`.

### HEAL-CORE (stages: core, shaman)

- **Proc type mask:** a hit that deals and heals nothing (a hot or debuff landing, not a tick) procs only a
  `spell_proc` row whose `SpellTypeMask` has NO_DMG_HEAL (0x4), per [ac] `SpellMgr::CanSpellTriggerProcOnEvent`.
  `SpellTypeMask` is already in serverdata; carry it in `core.ServerProc` and check it where server procs fire. Today
  `landHot`'s 0-damage hit procs Pandora's Plea, Show of Faith and Sif's Remembrance.
- **Talisman of Troll Divinity** (`sim/common/wotlk/other_effects.go`, 37734): 60518 stacks as on the server, so
  its flat healing taken applies.
- **Enemy-only spells at a friend:** check that damaging spells and on-use items cast at a healer's friendly current
  target (priest SW:P, shaman shocks, paladin Exorcism, item MCDs) neither crash nor fire, with a core guard if one
  is cheap.
- **Resto Shaman APL** (stage shaman): the default APL drops its Healing Wave line. Re-measure on the page's
  request at 300 s, per phase (time to OOM, HPS, "Measure at my gear"), as HEAL-FIGHT did; the guide's numbers
  follow.
- Full suite: report every suite that moves, DPS ones included, with the procs or stacks behind it.
- **Owns:** `sim/core` proc code, `sim/common/wotlk/other_effects.go`'s talisman, the guard, `ui/restoration_shaman/apls/`,
  `sim/shaman/restoration/` tests, the Resto Shaman numbers in the guides.

#### As built

Stage core (the shaman stage adds its own lines below).
- **Spell type mask:** `ServerProc.SpellTypeMask` and `ProcTrigger.SpellTypeMask` (`ProcSpellID` fills it);
  `ApplyProcTriggerCallback` refuses an event whose type (`procEventSpellType`: heal, else damage, else
  `NO_DMG_HEAL`) the mask lacks, for hit, tick and heal callbacks, white swings excepted. Hand-rolled procs call
  `ServerProc.AllowsEvent`. Tests: `sim/core/proc_spell_type_test.go`. Sim hits carry no absorb, so that rule
  has no test.
- **Talisman of Troll Divinity:** a stack of 60518 per direct heal of a magic-class spell, 5 max
  (`TestTalismanOfTrollDivinityStacksOnDirectHeals`).
- **Enemy-only spells at a friend:** `Spell.dealsDamageToAFriend` in `CanCast`, `Cast` and the APL gate: a
  non-helpful, non-healing spell with a damaging proc mask or an on-use item, with damage, at another friendly
  unit; area spells pass. Tests `TestNoDamageCastsAtAFriend` (shaman, paladin), `TestNoShadowWordPainAtAFriend`.
- **Goldens (not promoted):** 12 of 37 suites move. Healer suites (Resto Druid, Holy Paladin, Disc and Holy
  Priest, Resto Shaman): only the Talisman row (+0.7 to +1.4% HPS) and the Gnomish Lightning Generator row (no
  longer damages the dummy). DPS suites move by 0.01 to 0.3% (single outliers to 0.8% on short runs, RNG shifts):
  an item proc whose `spell_proc` row is damage only (`SpellTypeMask` 1) no longer rolls on a landing that deals
  nothing. Flare of the Heavens: Balance (Insect Swarm, Starfall) and Mage (Living Bomb, Arcane Missiles).
  Mjolnir Runestone: Feral (Rip, Faerie Fire) and Rogue (poisons, Expose Armor, Rupture). Comet's Trail: Rogue.
  Swordguard Embroidery (damage only entry, hand-rolled) now asks `AllowsEvent` too; no suite equips it, so no
  golden moves. No on-hit trinket or enchant gains procs.
- Not covered (outside this item's paths): hand-rolled procs in `sim/common/tbc` (Hand of Justice, Dragonspine
  Trophy, Madness of the Betrayer, Fists of Fury, Twin Blades) and the paladin seals roll on any landed hit.

Cross-review (wave HL3, uncommitted):
- The mask reached only 17 `ProcSpellID` items. Now 44 more item procs take their equip spell's entry (numbers
  equal to the hard-coded ones, so only the mask moves them): Meteorite Whetstone, Grim Toll, Banner of
  Victory, Deathbringer's Will, Ashen Band of Destruction, Tiny Abomination, Heartpierce, Death Knight's Anguish,
  the `ProcDamageEffect` trinkets and others; Shadowmourne, the tbc trinkets, bands and sets, the seals, Sword
  Specialization, Sudden Death, Hack and Slash, Bloodworms and Aspect of the Viper call `AllowsEvent` (the last
  two also skipped misses). `gen_serverdata` rerun: 50 proc rows, additions only.
- A cast or finish phase entry never filters (its events take every type); Auto Shot is checked, melee swings
  aren't. Lifebloom's bloom (`SPELL_ATTR3_SUPPRESS_CASTER_PROCS`) now procs nothing, Talisman stacks and Omen
  of Clarity included (`SpellFlagNoOnDamageDealt` covers heals). Innervate gives 786 a tick (int32), not 786.6.
- Goldens (not promoted, with the paladin and druid fixes): 19 of 37 suites move. Restoration Average-Default
  HPS -1.7% (the bloom's Omen and procs, Innervate); Arms -0.11% (Sword Specialization and Sudden Death off Rend
  and Sunder landings), Fury -0.03%, Feral -0.007% (Grim Toll off Rip and Faerie Fire), Enhancement +0.016%, BM
  and SV about 0; Survival NoBuffs rows -1 to -2.2% (Viper mana off misses); single AllItems rows to -3.1% (Tiny
  Abomination off judgements on Retribution). Blood, Blood Tank, Feral Apl and Tank, MM, Holy, Protection,
  Assassination, Combat, Subtlety and Protection Warrior: AllItems, set or talent rows only.

Stage shaman.
- `ui/restoration_shaman/apls/default.apl.json` drops its Healing Wave line (Tidal Waves condition, 49273).
  `TestDefaultAPLHeals` now asserts Healing Wave is never cast. Healing Wave stays a registered spell.
- Re-measured on `pageRequest` (300 s, 3000 iterations, seed 424242, `statslopes.Measure`), after the core stage.
  With the line (HEAL-FIGHT's table, reproduced exactly after the core stage) vs without; time to OOM (s out of
  mana) and HPS:

  | | preraid | P1 | P2 | P3 | P4 | P5 |
  |---|---|---|---|---|---|---|
  | before | 111 (118) 5273 | 130 (91) 7265 | 102 (125) 8537 | 147 (84) 10550 | 181 (65) 16827 | 165 (83) 16230 |
  | after | 178 (75) 6553 | 255 (27) 9234 | 171 (78) 9407 | 211 (48) 12099 | 283 (15) 20662 | 241 (37) 20444 |

  "Measure at my gear", HPS a point, gain/loss:

  | | Int | Spirit | SP | MP5 | Crit | Haste |
  |---|---|---|---|---|---|---|
  | preraid | 4.20/3.63 | 0.23/0.14 | 1.96/1.97 | 6.04/5.51 | 2.27/2.07 | −0.08/0.01 |
  | P1 | 4.20/5.17 | 0.09/0.07 | 2.48/2.47 | 5.57/7.04 | 2.99/3.12 | −0.08/0.46 |
  | P2 | 5.13/5.01 | 0.29/0.20 | 2.18/2.17 | 8.28/7.12 | 3.13/2.99 | −0.15/0.37 |
  | P3 | 5.93/5.79 | 0.06/0.05 | 2.69/2.64 | 8.61/9.22 | 4.00/3.81 | −0.53/−0.24 |
  | P4 | 7.55/10.26 | 0.02/0.03 | 6.37/6.76 | 7.95/13.60 | 7.38/8.32 | 2.76/4.92 |
  | P5 | 11.44/12.34 | 0.17/0.18 | 5.66/6.25 | 15.29/15.59 | 9.42/9.28 | 2.17/2.37 |

  Spirit about 0 (five-second rule) and haste at or below 0 preraid to P3 (mana-bound) are explained; the rest
  are nonzero.
- Goldens (Resto Shaman suite, not promoted): `Rotation-Default-Average` (180 s, 9 dummies) +14.7% HPS, Chain Heal
  casts 31 -> 75, Healing Wave gone. Every row on one dummy falls 10 to 14% (the AllItems, Settings and
  SwitchInFrontOfTarget rows; short fights to -36%): Chain Heal has no second target there and Healing Wave was
  the better single-target heal. Ancestral Awakening casts fall (24 -> 13 in `Average-Default`).

### HEAL-PAL-2 (G; stages: items, page, live)

- Holy glyphs, T7 to T10 holy sets (T8 4p shortens Sacred Shield's cooldown, T9 4p raises 66922; both read in the
  [ac] scripts), healing librams.
- APL: Beacon of Light on another unit than the heal target (a beacon on it copies nothing), Sacred Shield, Holy
  Shock on cooldown, Holy Light or Flash of Light filler, Divine Illumination, Divine Plea, judgements on `Target`.
  Presets, launch.
- **Owns:** the paladin's holy glyphs, sets and librams, `ui/holy_paladin/`, `sim/paladin/holy/`.

#### As built

Stage 1, items (uncommitted):
- Glyphs, sets and librams in `sim/paladin/holy_items.go`, rules in [class-mechanics](../guide/class-mechanics.md#paladin)
  and [damage and healing](../guide/damage-and-healing.md#healing); tests `sim/paladin/holy/items_test.go`.
  - Glyphs: Holy Light (54968 on five others), Flash of Light, Holy Shock, Beacon of Light, Seal of Wisdom, Seal of
    Light, the Wise. Divinity, Divine Plea and Lay on Hands were in. Cleansing and Salvation do nothing here.
  - Sets T7 to T10 (Redemption Regalia, Aegis Regalia, Turalyon's / Liadrin's Garb, Lightsworn Garb), both bonuses
    each; T8's Holy Mending 64891, T9's 66922 doubling, T10's Holiness 70757.
  - Librams: Renewal, the Resolute, Mostly Holy Deeds, Veracity, Blinding Light. The TBC librams aren't in
    `db.json`, so none is worn. Libram flats go after every percent (`flatHealing`).
- `HasSetBonus` panics once the sim is built, so the bonuses the casts read are cached at registration.
- Needs a live capture (stage 3): 54968 (data from Spell.dbc: heal, 5 targets, no crit). `gen_serverdata` rerun,
  additions only. 71166 (T10 2 piece, +35% healing done) is in neither the dump nor the sim by id; its value is from Spell.dbc.
- Also touched: `sim/optimizer/residuals.go` (5 libram ids), `serverdata_undeclared.go` (64891), `healSetup` (glyphs,
  extra parties for more than four dummies).
- Goldens: `TestHoly` Average-Default HPS 9974 to 11711 (+17.4%), TPS +10.6%, DPS -0.1%, from the standard glyphs:
  Holy Light's 54968 (+90766 total), more Holy Light casts from Seal of Wisdom's cost cut (Flash of Light fewer),
  Beacon at 90 s. Four new `AllItems` rows (the sets) in Holy, Protection and Retribution.

Stage 2, page (uncommitted):
- Default APL `ui/holy_paladin/apls/default.apl.json` as `autoRotation` and the one rotation preset (rule and numbers in
  [class-mechanics](../guide/class-mechanics.md#paladin)). Presets P1 to P4 kept (all in the server catalog by tier),
  P5 added (Ruby Sanctum cloak, bracers, belt, boots, ring, trinket); Libram of Renewal stays in all (it out-heals the
  later librams in HPS). Talents and glyphs kept; Runic Mana Injector as the potion; page buff and debuff defaults cut
  to the shaman's; EP weights from the P1 slopes. Launched; `header.spec.ts` lists it in the menu and in the
  no-alert test, new `tools/uitest/tests/settings/holy_paladin_page.spec.ts` runs a sim from the page (HPS > 1000).
- `default_apl_test.go`: the page's request (9 dummies, 300 s) heals with every APL spell, P1 runs dry inside the
  fight, every preset item is in the server catalog by its phase's tier. `TestHoly` runs the page APL (rows keep
  their names); `statslopes`' Holy Paladin row runs it with `heals: true`.
- Page request at 300 s (time to OOM s, s out of mana, HPS; HL3 cross-review, merged tree): preraid 127 (64) 9.9k,
  P1 151 (51) 12.3k, P2 205 (28) 17.9k, P3 226 (21) 20.6k, P4 none (1) 28.7k, P5 252 (12) 29.3k; HEAL-CORE moved
  them by at most 1 s and 0.1%. "Measure at my gear" gains are nonzero except
  Spirit (0 to 0.11) and haste (about 0 or below while mana-bound: P1, P5).
- Goldens, `TestHoly` Average-Default against base: HPS 9974 to 11894 (+19.3%), DPS +0.9%, TPS +8.4%. Stage 1's glyphs
  account for most of it; the page APL drops the Flash of Light filler (53653 and 48785 gone, 66922 with it) for
  116 Holy Lights (+54968 on five others) and casts Divine Plea under 60% mana.

Stage 3, live (uncommitted; server lock held 13:57 to 14:07 UTC, no rebuild):
- Spell capture: 54968 captured (`TestSpelldumpCapture`) into `spelldump.jsonl` (additions only); `gen_serverdata` rerun
  with no arguments; `spellids` reports only the two known unknowns (26545, 53258). The capture matches stage 1's
  declaration: attribute `IGNORE_CASTER_MODIFIERS`, no coefficient, class mask 0, not crit capable.
- Timing: a private copy of the e2e harness (not committed) took `.simval timing` of 12 holy spells and 7 new auras and
  heals (187 records): bare; under each glyph (Holy Shock, Beacon, Seal of Wisdom with and without the seal up, Seal of
  Light, the Wise), T7 to T10 bonus auras, Holiness, Libram of Renewal, and Holiness under Bloodlust. Replayed with the
  set pieces and libram worn on the sim character. Every cast time, GCD, cooldown, cost and aura duration agrees:
  Glyph of Holy Shock 5 s, Beacon 90 s, Seal of Wisdom -5% on Holy Light 1210, Flash of Light 291, Holy Shock 750 and
  Beacon 1460 only while the seal is up, the Wise's Seal of Wisdom 307, T7 Holy Light 1210, Renewal 1161, T9 Judgements
  of Light and Wisdom 30 s, Holy Mending 3 ticks of 3 s, Holiness 10 s, Veracity 15 s, Blessed 15 s.
  - Holiness: Holy Light 2200 ms (1692 under Bloodlust) on the server. `Spell.CastTime()` doesn't run `ModifyCast`, so
    the replay can't read it; `TestLightswornGarb4PieceHoliness` casts it and agrees.
  - Not reachable by a probe: T8's 4 piece (a dummy aura read by the Sacred Shield script) and the libram flat heals.
  - A probe of a seal takes the seal aura off, so probe it after the spells that need it up.
- Talent and spell-mod sweeps not re-recorded: no talent or rank spell changed, glyph and set effects aren't in
  `spellmod_spells.json`, and the committed `TestTalentSweepRecords` and `TestSpellModRecords` pass.
- Fixed: nothing disagreed. Stage 1's comment on 54968 said "no damage class"; the capture shows a magic class, with
  the ignore-caster-modifiers attribute doing the work. Comment corrected.

HL3 cross-review (uncommitted):
- Holiness (70757) has one charge (`spell_proc`: `Charges` 1, `PROC_ATTR_REQ_SPELLMOD`): only the next Holy Light is
  0.3 s faster, not every one for 10 s. Fixed in `holy_items.go`; `TestLightswornGarb4PieceHoliness` checks the
  second Holy Light. No preset wears T10's 4 piece.
- 54968's flat healing taken coefficient is an area heal's, 0.75 s of 3.5 × 1.88, not an instant's
  (`TestGlyphOfHolyLightHealsFiveOthers`).
- Libram of Tolerance (40268, P1 tier, +141 Holy Light `SPELLMOD_DAMAGE`, 60792) was missing; added. Renewal still
  out-heals it and the Resolute at every phase.
- Page glyphs: the Wise for Sense Undead, the only minor glyph that does anything here (+3 to +16 HPS).
- Tests tightened: Veracity's 8 s cooldown, T8's 4 piece against the 7 s without it, the page APL's beacon copies,
  54968 and Divine Plea.
- Goldens (not promoted): `TestHoly` AllItems-LightswornGarb HPS +0.37% (Holiness), AllItems-TalismanofTrollDivinity
  -0.09% (54968's coefficient). Protection and Retribution unchanged.

### HEAL-DRU-2 (G; stages: items, page, live)

- Resto glyphs (Glyph of Swiftmend keeps the hot), T7 to T10 resto sets, healing idols.
- Innervate: `registerInnervateCD`'s `ExtraCastCondition` is inverted (from upstream), so it never casts; fix it
  before the page measures mana. Report Balance, Feral and raid suites that move.
- APL: Lifebloom stack, Rejuvenation and Wild Growth kept rolling, Swiftmend, Nourish filler, Innervate. Presets,
  launch.
- **Owns:** the druid's resto glyphs, sets and idols, `sim/druid/innervate.go`, `ui/restoration_druid/`,
  `sim/druid/restoration/`.

#### As built

Stage 1, items (uncommitted; numbers before HEAL-CORE):
- Innervate: `registerInnervateCD`'s `ExtraCastCondition` is no longer inverted. `TestInnervateCastsOncePerCooldown`
  (self and another player): one cast and 7860 mana per 3 minutes. No Balance, Feral, Tank or raid golden holds an
  Innervate target, so none moves from it; Restoration's does (self Innervate).
- Glyphs (`sim/druid/resto_items.go`, rules in [caster-mechanics](../guide/caster-mechanics.md#druid)): Healing
  Touch, Lifebloom, Nourish, Rapid Rejuvenation, Regrowth, Rejuvenation, Swiftmend, Wild Growth, Innervate. T7 to
  T10 sets (Dreamwalker and Nightsong Regalia, Malfurion's or Runetotem's Garb, Lasherweave Garb; T7's 4-piece
  60137 has no server code) and the resto idols (Budding Life, Awakening, Pure Thoughts, Lush Moss, the Tenacity
  idols of every season, Flourishing Life, Flaring Growth, Black Willow). One test each in `items_test.go`.
- Needs a live capture (stage 3): 54755 (Glyph of Rejuvenation's heal), then `gen_serverdata`. It is built like
  Glyph of Healing Wave's 55533 (no coefficient, can crit); its attributes are unchecked.
- Also touched: `sim/optimizer/residuals.go` (the idol ids), `sim/core/serverdata` (regenerated, additions only).
- Goldens: Balance, Feral and Tank gain `AllItems` rows for the four resto sets. Restoration gains the same rows
  and moves everywhere: Average-Default HPS 6079 to 7471 (+22.9%) from Innervate (more Nourish, Lifebloom and
  Healing Touch casts), Glyph of Nourish (+6% a hot on Nourish: 7904 to 9586 a hit), Swiftmend's glyph (Rejuvenation
  stays up) and Wild Growth's 6th target.

Stage 2, page (uncommitted; numbers before HEAL-CORE):
- Default APL `ui/restoration_druid/apls/default.apl.json` as `autoRotation` (rule in
  [caster-mechanics](../guide/caster-mechanics.md#druid)); `TestRestoration` and the stat-slopes row run it. Self
  Innervate is the page default; P5 preset (P4 plus the Ruby Sanctum cloak, bracers, belt, ring and trinket); talents
  and glyphs kept. Page buffs and debuffs match the shaman page's, so no bloodlust. Launched; `header.spec.ts` now
  lists the druid page in the menu and checks its healing note. The page had no alert.
- Tests (`default_apl_test.go`): the page's padded 9-dummy request at 300 s heals with every APL spell, runs P1 dry
  inside the fight, and every preset item is in the server catalog by its phase's tier.
  `TestHealerIsMeasuredInHPS` has `heals: true` for the druid.
- Page request, 300 s (time to OOM, HPS): preraid 126 s 7000, P1 174 s 9189, P2 to P5 none, 13532 / 13421 / 20836 /
  21081. "Measure at my gear" (Int / Spirit / SP / crit / haste / MP5): P1 4.07 / 2.49 / 2.35 / 0.37 / -0.44 / 6.53;
  P3 0.52 / 0.73 / 2.79 / 0.72 / 0.09 / 0.35; P5 0.99 / 1.28 / 3.09 / 0.79 / -0.21 / 0.86. Haste is near 0 or
  negative while mana-bound, as the build only spends mana sooner.
- Goldens (against the base, items and page together): `TestRestoration` Average-Default HPS 6079 to 7748 (+27.4%),
  Settings rows +13% to +59%; the other druid suites move only by stage 1's new set rows.

Stage 3, live (uncommitted; server lock held 13:40 to 14:03 UTC, no rebuild; numbers before HEAL-CORE):
- Captured 54755 (`TestSpelldumpCapture`), added to `spelldump.jsonl`; `gen_serverdata` rerun (additions only);
  `spellids` reports only 26545 and 53258. 54755 is `SPELL_ATTR2_CANT_CRIT` with `IGNORE_HEALTH_MODIFIERS`: the
  sim's glyph heal no longer crits (it was built as Glyph of Healing Wave's 55533).
- `tools/acore/spellmods` regenerated: the T8 heal 64801 joins the spells talents' mods reach, Blessing of the
  Eternal's 70691 is excluded (ignores caster modifiers). The spell-mod sweep over the four druid paths
  (`SIMVAL_SPELLMOD_ONLY`) replaced their lines in `spellmods.jsonl`; `TestSpellModRecords` passes (same two
  disagreements as before, both already dropped). Glyphs, sets and relics are not talents, so the sweep doesn't
  reach them.
- Timing: a private copy of the e2e harness (not committed) took 208 `.simval timing` records of Healing Touch,
  Lifebloom, Rejuvenation, Nourish, Regrowth, Wild Growth, Swiftmend and Innervate: bare, with each resto glyph,
  T7 to T10 bonus spell and relic spell alone, all together (plus Bloodlust), and the three timing glyphs with
  and without Bloodlust. Replayed through `replayTiming` (sets and relics worn as the items): 1224 checks
  agree. Server values: Glyph of Healing Touch cast 1500 ms, cost 864; Glyph of Lifebloom 8 s; Rapid
  Rejuvenation divides the hot's 15 s by haste (11538 ms at Bloodlust); T7 2-piece Lifebloom 929; Budding Life
  -36 and Awakening -106 on Rejuvenation's 629, adding to 487; the other glyphs, T8 to T10 spells and relics
  move no timing. The all-together records can't be replayed (a character has 3 glyphs); they add up as the
  single ones do.
- Fixed from it: Innervate costs no mana on the server (the sim took 4% of base mana, 139); the cost is gone.
  `TestInnervateCastsOncePerCooldown` holds it at 0.
- Goldens, `TestRestoration` against the base: Average-Default HPS 6079 to 7766 (+27.7%), LivingSeed-Average
  6236 to 7913, NoBuffs LongSingleTarget 2918 to 4717; Innervate's cost removal adds under 0.3% to stage 2's.
  Balance, Feral and Tank: new set rows only.
- Left: T7's 4-piece (60137) has no server code; Glyph of Rejuvenation 54754, Blessing of the Eternal and the
  Flaring Growth and Black Willow idols are modelled from the scripts and `spell_proc` rows, not measured live
  (the harness probes no heals' amounts).

Cross-review (wave HL3, uncommitted; numbers on the merged tree, after HEAL-CORE):
- Fixed against [ac]: Blessing of the Eternal took the druid's spell mods (it ignores caster modifiers), had 5
  ticks with Nature's Splendor (6) and no Rapid Rejuvenation haste, and never went on the healed unit (the cast
  has no explicit target); Swiftmend now eats it and Revitalize rolls on it. T8's 64801 crit (it can't) and
  missed Master Shapeshifter. Pure Thoughts', Lush Moss' and the Flourishing Life's flats went before the
  percents (the last as healing power). T10's Wild Growth decay now loses 30% again at each refresh. T9's tick
  crit rolls the chance snapshotted at apply. 54755 took flat healing taken.
- Goldens (not promoted): `TestRestoration` AllItems-NightsongRegalia HPS +0.068%, AllItems-LasherweaveGarb
  +0.378%; the other 5 druid suites unchanged. Balance and Feral with self Innervate never reach its threshold
  (P1 to P3, 180 and 300 s), so they don't move.
- Page request (3000 iterations, seed 424242): preraid 128 s 6938, P1 174 s 9078, P2 to P5 12375 / 13425 / 20413
  / 20680 (average time to OOM past 300 s). "Measure at my gear": P1 4.10 / 2.55 / 2.32 / 0.28 / -0.43 / 6.77;
  P3 0.52 / 0.73 / 2.79 / 0.73 / 0.05 / 0.37; P5 1.16 / 1.42 / 3.77 / 1.08 / 0.17 / 0.96. HEAL-CORE moved none of
  stage 2's numbers past noise. The druid fixes moved P2 to P5 by -0.2% to +0.3%; the bloom procing nothing
  (HEAL-CORE's cross-review) and Innervate's 786 a tick then took 1 to 3% off, P2 9% (fewer Val'anyr bubbles).

### HEAL-CAPS (stages: code)

- The Caps tab's haste cap (`sim/statcaps`, `spellHaste`) counts only always-on haste. For a priest with Borrowed
  Time, its note also gives the cap right after Power Word: Shield: bisected with the buff's cast speed (+5% a
  point) on the sheet, and says that any magic-class spell but Penance and Prayer of Mending uses the buff up
  ([caster-mechanics](../guide/caster-mechanics.md#priest)). The Disc page's defaults read 1012 and 154; a
  `statcaps` test holds both.
- Golden-neutral.
- **Owns:** `sim/statcaps/compute.go`'s haste note, `sim/statcaps/statcaps_test.go`, the haste ceiling facts in
  `docs/guide/stat-caps.md`.

#### As built

- `borrowedTimeNote` (`sim/statcaps/compute.go`) extends the haste note of a `HealingPriest` with Borrowed Time (talents
  parsed with `priest.TalentTreeSizes`): the cap bisected with cast speed divided by 1 + 5% a rank. The row's
  `needed` stays the always-on cap.
- Disc page defaults (Undead, 5/5): 1012 always-on, 154 after a shield (P1 gear; always-on 1012 at every phase). A Holy
  talent string gets no extra text. `TestDiscHasteNoteGivesTheCapAfterShield` holds both.
- `docs/guide/stat-caps.md` names the extra note. Golden-neutral: no suite reads the note.

### HEAL-EFFECTIVE (G, FS; stages: core, priest-paladin, druid-shaman, measure)

- **Wrath of Air** on by default on the Resto Druid and Holy Paladin pages and in their page-request tests
  (`default_apl_test.go`), as on Disc and Resto Shaman.
- **Dummies** on a healer request (`TargetDummies`) get health bars. Dummy 1 is the tank, the other eight the raid.
  Max health: tank 40000 and raid 20000 at preraid and P1, +5000 and +2500 a phase after (P5 60000 / 30000). A
  dummy never dies: damage stops at 1 health.
- **Incoming damage:** a damage-per-second setting on healer pages (in the request beside `target_dummies`). Half
  hits the tank every 2 s; the other half hits four of the eight raid dummies, picked at random, every 4 s. No
  mitigation; shields don't absorb it. The hits are hits taken, so effects waiting on one see them.
  - Default per spec and phase: the page request's HPS at that phase's preset when nothing overheals (damage
    high enough), rounded. The page takes it from its phase (`sim.getPhase()`) until the user sets it; Restore
    Defaults resets it.
- **Effective healing:** a heal on a dummy counts up to its missing health, the rest is overheal. On a healer
  request, unit and raid HPS are effective healing plus shielding (shields in full, as today); raw healing,
  overheal and the damage left unhealed are their own metrics (next free `UnitMetrics` numbers). DPS and tank sims
  don't change: no DPS or tank golden moves. A healer page's results show effective HPS, raw HPS and overheal %.
- **Stat values:** `statslopes`' healer role measures effective HPS.
- **APL:** add what the healer rotations need and lack, listed in the APL editor: a heal on the most hurt friendly
  unit (lowest health percent, ties by raid order), a HoT on a hurt unit that lacks the caster's, a buff, HoT or
  shield kept up on the tank, and a missing-health condition.
- **Rotations:** each healer page's default APL (Disc, Holy priest, Holy Paladin, Resto Druid, Resto Shaman) heals
  with the class's usual priority on the most hurt dummy, big heals only on a unit missing about that much, and
  waits when nobody is hurt. Tank upkeep (Beacon, Sacred Shield, Lifebloom, Earth Shield) and Prayer of Mending may
  go out ahead of damage.
- **Stand-ins:** effects waiting on a hit taken (Prayer of Mending's 5 s proc, Earth Shield's charges, Living
  Seed, Sacred Shield, the other `core.HitTakenStandIn` users) use the dummies' hits; Chain Heal's jumps, Ancestral
  Awakening, Wild Growth and Blessing of the Eternal pick hurt units as [ac] does. `HitTakenStandIn` stays only for
  a request with no incoming damage.
- **Goldens:** the four healer suites move (raw and effective apart); their rows show effective HPS. No other suite
  moves.
- **Measure** (last stage), per spec and phase on the page request at 300 s, 3000 iterations, seed 424242: the
  default damage re-measured with the new APLs (updated if it moved over 5%), time to OOM, effective and raw HPS,
  overheal %, damage left unhealed, "Measure at my gear" gain and loss. Compare each spec's stat order with the
  class guides (Wowhead's: Resto Druid crit low, Resto Shaman haste over crit, Spirit near 0 for shamans) and say
  where and why they differ; don't tune the model to fit them. These replace the guides' healer numbers.
- Stages: core (Wrath of Air, dummies, damage, effective metrics, APL additions, `statslopes`, the page setting and
  results, the defaults measured with today's APLs); priest-paladin and druid-shaman (rotations and stand-ins);
  measure.
- **Owns:** `sim/core`'s target dummy, health and healing metrics, the request field and its UI, the APL
  additions, `sim/statslopes`' healer role, the four healer pages' `ui/` APLs, defaults and raid buffs, the classes'
  stand-in code, the healer suites and their tests, the healer numbers and rules in the guides
  (caster-mechanics, class-mechanics, damage-and-healing, stat-caps' healer values).
