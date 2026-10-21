# Spec balance: P4 proposal

One server tweak per build still below B at the baseline (PLAN, "In-scope builds and baseline"), for the user's
review in one sitting; only approved tweaks go to P5. Each was prototyped in a per-class copy of the sim and
measured with the rankings runner (P1–P5, 180/300 s, the baseline's captures, seed 424242, 10k iterations), then
all together in one copy, `tmp/p4/combined/`: only the 15 targeted builds move, every other build stays
identical to the digit. Prototype diffs: `tmp/p4/patches/<class>.patch` (gitignored; values hard-coded, P5
redoes them per PLAN P5).

**Ranking** below means the build's worst and best % of the top build over the 10 counted cells (P1–P5 at 180
and 300 s). B tier needs 85% in every cell; a server buff must keep every cell under 95%.

## Patch notes

Talent text gives the new values; "(was …)" gives today's. A talent's other effects stay as they are.

### Warrior

**Arms** (recommended, after the gear check in decision 2)
- **Endless Rage** also makes your attacks ignore 36% of the target's armor (was: +25% rage from damage dealt
  only). It adds to Battle Stance's 10% and your armor penetration rating, under the same cap.
- **Wrecking Crew:** the Enrage from your melee crits increases physical damage by 5/10/14/19/24% (was
  2/4/6/8/10%).
- Why: Arms' damage is physical hits into armor. Early gear has little armor penetration, so Arms falls
  furthest behind at P1–P2. The armor part stops mattering once gear reaches the armor penetration cap (P3 on);
  the Enrage covers the ~10% gap left in every phase.
- Ranking: 69.9–81.7% → 86.4–93.2%.

**Fury** (not recommended, decision 3)
- **Unending Fury** would also make your attacks ignore 3/7/10/14/17% of the target's armor (today: Slam,
  Whirlwind and Bloodthirst +2–10% damage).
- Ranking: 80.2–100% → 85.1–100%, but Fury would pass BM as the top build at P4/300.

### Paladin

**Retribution** (closest result: 4 cells stay below B)
- **Improved Judgements** also gives your melee auto attacks a 20/40% chance to reset Divine Storm's cooldown,
  the T10 2-piece bonus's effect. It does nothing while you wear the T10 2-piece, so the two don't stack (was:
  Judgement cooldown −1/−2 s only).
- **Swift Retribution** also adds 400/800/1200 base damage to each Consecration tick: 113 → 1313 at 3/3, about
  four times the damage per tick with spell power (was: your auras give +1/2/3% haste only).
- Why: Ret's jump at P4 comes from the T10 2-piece (twice the Divine Storms) and two P4 items. Before that it has
  idle GCDs with everything on cooldown. The reset brings T10's effect to P1–P3; the stronger Consecration fills
  the idle GCDs.
- Ranking: 68.7–88.4% → 80.9–93.7%. P1/180, P2/180, P2/300 and P3/180 stay at 81–82%.
- Side effect: Consecration becomes a much stronger AoE on trash too.

### Rogue

**Combat** (recommended)
- **Lightning Reflexes** gives 9/16/23% melee attack speed (was 4/7/10%). Dodge unchanged.
- Why: Combat's damage is white swings, poisons that proc on hits and Rupture (which the module already speeds
  up with haste), and all three grow with attack speed. Its gap is the same in every phase.
- Ranking: 80.9–84.6% → 88.0–92.2%.

### Hunter

**Marksmanship** (recommended, after the gear check in decision 2)
- **Marked for Death** also makes Auto Shot, Aimed Shot, Multi-Shot, Steady Shot, Kill Shot and Silencing Shot
  ignore 15% of the target's armor per rank, 75% at 5/5 (was: +1–5% shot damage on marked targets and +2–10%
  crit damage on some shots only). It applies before armor penetration, so it helps a lot while gear has little
  armor penetration and almost nothing once gear reaches the cap (P4/P5). Pet attacks don't get it; it works
  against players too.
- Why: MM's shots are physical and its early gear lacks armor penetration.
- Ranking: 78.9–92.6% → 86.4–93.7%.

**Survival** (recommended)
- **Explosive Shot** deals double base damage per tick: 772–928 at rank 4 (was 386–464). The attack power part is
  unchanged, so this matters most with weak gear. Blizzard used the same lever for Season of Discovery hunters
  (+15%, then +50%).
- Ranking: 81.1–88.5% → 86.6–93.6%.

### Mage

**Arcane** (recommended)
- **Arcane Empowerment:** Arcane Blast and Arcane Missiles gain an extra 5/10/15% of your spell power (was
  3/6/9%). The 3% raid damage buff is unchanged.
- Why: Arcane's gap is the same in every phase, and these two spells deal 98% of its damage.
- Ranking: 81.1–85.3% → 87.4–91.8%.

**Fire** (not recommended, decision 4)
- **Living Bomb** deals 12% more base damage: tick 386, explosion 772 (was 345 and 690).
- Ranking: 77.5–97.4% → 77.8–97.5%. Each cell moves only 0.3–0.4.

### Warlock

A crit normally deals 209% damage (with the meta gem). The crit damage tweaks below raise that number.

**Affliction** (recommended)
- **Empowered Corruption** also makes Shadow Bolt crits hit harder: 232% damage at 3/3, +14% crit damage bonus a
  rank (was: extra spell power for Corruption only).
- Why: Shadow Bolt's crit chance grows with gear, so this helps late phases most, where Affliction falls behind.
- Ranking: 83.8–92.7% → 86.0–94.1%.

**Demonology** (closest result: P5 stays below B)
- **Nemesis** also makes Shadow Bolt, Incinerate and Soul Fire crits hit harder: 225% damage at 3/3, +10% crit
  damage bonus a rank (was: shorter cooldowns on Demonic Empowerment, Metamorphosis and Fel Domination only).
- Why: the Felguard doesn't get the warlock's crit or haste, so its share shrinks as gear improves. No lever found
  grows fast enough with gear to fix P5 without pushing P1/300 past 95.
- Ranking: 80.3–91.9% → 83.6–94.4%. P5 stays at 84%.

**Destruction** (closest result: P4 and P5 stay below B)
- **Pyroclasm:** the buff from Searing Pain and Conflagrate crits increases Fire and Shadow damage by 7/13/20%
  (was 2/4/6%).
- Ranking: 74.1–88.8% → 80.6–94.3%. P4 and P5 stay at 81–85%. Destruction's P4 BiS gear lacks the T10 4-piece
  (+4.5% when fixed, below).

### Shaman

**Elemental** (recommended)
- **Call of Thunder** also makes Lightning Bolt and Chain Lightning crits, Lightning Overload copies included,
  deal 269% damage instead of 209% (was: +5% crit chance for both only).
- Why: Elemental's late gear is mostly crit and haste, and nothing in the spec turns extra crit into extra
  damage beyond plain crits.
- Ranking: 75.0–83.2% → 86.8–93.5%. On the live server it may run 1–2% higher than the sim predicts, because the
  server makes each Overload half of the main bolt's damage, crit included.

**Enhancement** (recommended)
- **Weapon Mastery** increases weapon damage by 12/24/36% (was 4/7/10%). Spells, Lava Lash, the wolves and the
  Fire Elemental don't get it.
- Why: Enhancement's physical damage grows with gear, its spell damage barely does (its spell power is tied to
  attack power), so the physical part is where a buff grows with the phases.
- Ranking: 76.3–86.0% → 86.2–93.7%.

### Priest

**Shadow** (recommended)
- **Twisted Faith** (5/5) gives spell power equal to 53% of your Spirit (was 20%). Ranks 1–4 and the Mind Flay
  and Mind Blast bonus are unchanged.
- Why: Shadow lags with weak gear, and early BiS gear carries more Spirit than late gear, so this helps P1–P3
  most.
- Ranking: 81.9–91.9% → 85.8–94.0%. Spirit becomes worth about as much as spell power for Shadow, so bots will
  favor Spirit gear once stat weights are regenerated.

### Druid

**Feral** (recommended)
- **Primal Gore** also lets Rake's damage over time crit (was: Lacerate and Rip only). The T10 4-piece already
  does this, so P4/P5 don't change.
- **Rend and Tear:** Ferocious Bite gets +10/20/30/40/50% crit chance on bleeding targets (was +5–25%). Late gear
  already crits with every Bite.
- **Idol of the Ravenous Beast** (the P1/P2 BiS relic) adds 304 damage to Shred (was 203).
- Why: Feral sits at 94.9% at P4/300, so only changes that do nothing with late gear can be used; these three
  help P1–P3 only.
- Ranking: 80.1–94.9% → 86.0–94.7%.

## Decisions for the user

1. **Extra effects on existing talents.** Seven tweaks give a talent an effect it doesn't have today, such as
   Endless Rage also ignoring armor or Call of Thunder raising crit damage: Arms, Fury, MM, Elemental,
   Affliction, Demonology and Ret. The server already supports each effect type, so it's still SQL only, no new
   spell and no new code. Changing only existing numbers was tried first and doesn't fix these builds. The PLAN
   says "no new spells or mechanics". Affliction and Demonology also borrow talents unrelated to the spell they
   buff (Empowered Corruption, Nemesis), picked because only that spec takes them. *Recommended: accept.*
2. **Gear will shift for Arms, Fury and MM.** Their tweaks ignore armor, so the armor penetration gems in their
   current BiS become partly wasted. The next BiS run, or the bots' regenerated stat weights, will swap them
   for Strength or Agility, and the builds will end up stronger than measured here: Arms about 98–99% at
   P3–P5/300, Fury 2–4% above BM, MM about 96% at P3/300, all over the cap. *Recommended: before P5 ships
   them, rerun the BiS optimizer for these three builds with the tweak in the sim and re-size the numbers
   (MM's fallback: 13% a rank instead of 15%).* SV, Feral and Shadow were checked and hold; the other tweaks
   weren't re-geared.
3. **Fury.** Any buff big enough to lift its P1/P2 cells to 85% also makes it the top build at P4/300, ahead of
   BM by 0.2%, and decision 2 makes that worse. *Recommended: no Fury tweak; its P1/P2 cells stay at 80–83%.*
4. **Fire.** It is already at 97% at P4/P5/300, where the cap allows +0.3 points. Every lever measured lifts
   the late phases at least 2/3 as much as the early ones, so none fits. *Recommended: no Fire tweak.*
   Alternatives: Living Bomb +12% (each cell +0.3–0.4), or waive Fire's cap and double Living Bomb's base
   damage (P1–P3 83–87%, P4/P5 96–99.7%).
5. **Ret, Demonology, Destruction** get the closest result, with the cells listed above still below B.
   *Recommended: accept; rerun Destruction's P4 BiS first (about +3 points at P4).*
6. **Feral's idol** is an item change, not a talent. Only Feral can gain (Shred is a cat ability). Without it,
   P1/180 ends at 84.96. *Recommended: accept.*
7. **Fixes the cap doesn't limit** (end of this doc). *Recommended: apply in P5.*

## Predicted % of top, all tweaks together

With decisions 3 and 4 as recommended, only the Fury rows and the P4/P5 300 s tops change: BM keeps P4/300,
and every other P4/P5 300 s figure rises 0.1–0.2.

| Build | P1/180 | P2/180 | P3/180 | P4/180 | P5/180 | P1/300 | P2/300 | P3/300 | P4/300 | P5/300 |
|---|---|---|---|---|---|---|---|---|---|---|
| Arms | 88.5 | 86.9 | 86.4 | 89.4 | 89.4 | 93.2 | 87.4 | 90.2 | 92.8 | 91.3 |
| Fury | 85.1 | 86.7 | 92.0 | 97.8 | 99.5 | 88.3 | 86.0 | 94.5 | 100 | 100 |
| Retribution | **81.3** | **80.9** | **82.1** | 91.3 | 90.2 | 85.1 | **80.9** | 85.1 | 93.7 | 91.6 |
| Combat | 89.7 | 89.4 | 88.4 | 88.7 | 88.6 | 92.2 | 88.2 | 90.1 | 89.7 | 88.0 |
| MM | 91.4 | 88.1 | 91.7 | 91.4 | 91.5 | 93.5 | 86.4 | 93.7 | 92.5 | 91.1 |
| SV | 89.5 | 87.5 | 90.9 | 90.3 | 91.3 | 92.6 | 86.6 | 93.6 | 92.1 | 91.4 |
| Arcane | 91.1 | 90.3 | 89.7 | 91.4 | 90.8 | 91.7 | 87.4 | 90.8 | 91.8 | 90.1 |
| Fire | **80.6** | **80.0** | **83.8** | 94.6 | 94.1 | **81.6** | **77.8** | 85.1 | 97.5 | 96.8 |
| Affliction | 90.8 | 88.9 | 86.1 | 87.0 | 86.0 | 94.1 | 87.7 | 87.8 | 89.2 | 86.7 |
| Demonology | 92.1 | 88.8 | 85.3 | 86.2 | **84.2** | 94.4 | 86.9 | 86.4 | 87.0 | **83.6** |
| Destruction | 89.9 | 89.5 | 86.6 | **80.6** | **83.5** | 94.3 | 89.6 | 90.5 | **83.1** | **84.7** |
| Elemental | 92.4 | 93.4 | 92.8 | 87.8 | 88.5 | 92.4 | 89.7 | 93.5 | 88.1 | 86.8 |
| Enhancement | 92.3 | 91.8 | 93.0 | 87.7 | 87.4 | 92.8 | 89.4 | 93.7 | 88.2 | 86.2 |
| Shadow | 86.4 | 90.9 | 85.8 | 92.9 | 90.7 | 89.6 | 89.7 | 87.3 | 94.0 | 90.3 |
| Feral | 86.0 | 86.4 | 88.0 | 91.6 | 90.9 | 90.6 | 86.9 | 91.7 | 94.7 | 92.3 |

Bold: below B. Full check output: `tmp/p4/patches/combined-check.txt`. Margins to watch: P1/300's top is
Frost DK, riding the module's Epidemic disease-haste tweak; if BM took it, the warlocks' P1/300 cells rise about
0.6 (Demonology 94.95). MM's and SV's P2 cells depend on Assassination staying P2's top.

## Fixes the cap doesn't limit

- Combat: Lethality 5/5 instead of Vigor (Assassination tree `00532000514`), +0.16–0.41% in every cell: preset,
  presetgen and bots.
- Enhancement: Glyph of Stormstrike instead of Glyph of Flametongue Weapon (major 2), +0.3–0.5%:
  `ui/enhancement_shaman/presets.ts`.
- Ret, once its tweak is in: Consecration first in the APL and the bots' strategy, +3.7% at P1, +0.7% at P4
  (P4/300 about 94.6).
- Destruction's P4 BiS wears T9 2-piece + T10 2-piece; a T10 4-piece swap gains +4.45%: rerun its BiS.
- Shadow's P5 BiS sims below its P4 BiS (17873 against 17958): look in the BiS chain.
- Enhancement: Shamanistic Rage on cooldown with the T10 2-piece is unmeasured; the BiS skips T10 at P4/P5.

## Parity gaps found (not P4)

- Lightning Reflexes ranks 1/2: the sim has 3/6% haste, the server 4/7%.
- Demonic Pact: the sim's ICD is 1 s, the server's `spell_proc` 54909/53646 has 20 s since world update
  2026_03_16_00 (Demonology −0.03 to −0.21%).
- Lightning Overload's damage: the server takes half of the main bolt's dealt damage, the sim rolls its own
  half-damage bolt. Fix it before sizing Elemental's tweak (about 100% instead of 110%).

## Implementation details (P5)

All SQL: full-row `spell_dbc` overrides through [tweaks] `tools/dbc_export.py --generate-override`, static, no
config switch, server-only fields (tooltips go stale). Effect slots are 1-based as in the generator's columns;
"new effect N" means `Effect_N=6` (apply aura), `EffectDieSides_N=1`, `ImplicitTargetA_N=1` plus the listed
fields. Each server path was checked in the AzerothCore source; the sim model named matches it. Aura types: 79
damage done %, 107 flat spell mod, 108 % spell mod (misc 13 ignore armor, misc 15 crit damage bonus), 138 melee
haste, 174 spell power from a stat, 280 armor penetration %.

- **Arms.** 29623: new effect 2, aura 280, BasePoints 35; class mask B words stay 0. Sums with rating and Battle
  Stance under the ArP cap (`Unit::CalcArmorReducedDamage`). Enrage 57518–57522 `EffectBasePoints_1`
  1/3/5/7/9 → 4/9/13/18/23 (aura 79 physical). Sim `sim/warrior/talents.go`: `BonusArmorPenPct` +36 with
  Endless Rage; enrage table {.05,.10,.14,.19,.24}. Rejected: Sudden Death 20% (+1%), Endless Rage 75% rage
  (+1.8%), Wrecking Crew alone (P1/P2 miss).
- **Fury.** 56927/56929–56932: new effect 1, aura 280, BasePoints 2/6/9/13/16, `EffectSpellClassMaskA_1`
  33554432 → 0 (a stale Mortal Strike mask on the empty slot that would limit it to MS).
- **Retribution.** Improved Judgements 25956/25957: `ProcTypeMask` 0→4, `ProcChance` 101→20/40, new effect 2
  aura 4 (dummy) `EffectTriggerSpell_2` 70769; `spell_proc` row -25956 (ProcFlags 4, DisableEffectsMask 1, rest
  0); `conditions` rows source 24, entries 25956/25957, type 1 (aura) 70765, negated. Swift Retribution
  53379/53484/53648: new effect 2, aura 107, misc 3 (SPELLMOD_EFFECT1), BasePoints 399/799/1199,
  `EffectSpellClassMaskB_1` 0→32 (Consecration). Sim: `sim/paladin/talents.go` `applyDivineStormReset`,
  `sim/paladin/consecration.go` +400/rank. Confirm in game that slot 2 works beside `spell_pal_swift_retribution`
  (hooks the first aura 107) and the condition blocks the reset under T10 2P. Rejected: Holy Vengeance flat
  (worse shape); Exorcism without cooldown on Art of War +4600 (a cooldown change, always crits undead and
  demons, and under the bots' priority P4/300 ≈ 95.8).
- **Combat.** 13712/13788/13789 `EffectBasePoints_2` 3/6/9 → 8/15/22 (aura 138), generated with
  `--dbc-path A:/WOW/dbc/Changed`. Sim `sim/rogue/talents.go` table {1, 1.09, 1.16, 1.23}. Rejected: Savage
  Combat AP, Combat Potency 40% (energy, depends on the bot spending it).
- **Marksmanship.** 53241/53243–53246: new effect 3, aura 108, misc 13, BasePoints −16/−31/−46/−61/−76,
  `EffectSpellClassMaskC_1` 397313 (Auto, Multi, Aimed, Silencing), `C_2` 8388609 (Steady, Kill Shot); no 3.3.5
  spell uses misc 13 today. Sim: new `Spell.IgnoreArmorPct` (`sim/core/spell.go`, applied in
  `spell_resistances.go` `GetArmorDamageModifier` before the ArP cap), set in `sim/hunter/talents.go`. Rejected:
  +4%/rank armor pen (P3/300 95.8), flat Steady Shot (no shape).
- **Survival.** `EffectBasePoints_1/DieSides_1`: 53301 143/29 → 287/59, 60051 220/45 → 441/89, 60052
  324/67 → 649/133, 60053 385/79 → 771/157. Sim: serverdata 60052/60053, `sim/hunter/explosive_shot.go` tick
  ranges. SoD's exact ×1.725 gives P2/300 85.1.
- **Arcane.** 31579/31582/31583 `EffectBasePoints_1` 2/5/8 → 4/9/14 (aura 107, misc 24 bonus multiplier).
  Sim `arcane_blast.go`, `arcane_missiles.go` Flat 5×rank. Rejected: Spell Power crit damage (P2/300 85.2).
- **Fire.** Living Bomb 55360 `EffectBasePoints_1` 344 → 385 (keep `EffectBasePoints_2` 55361, the explosion
  id), explosion 55362 689 → 771.
- **Affliction.** 32381–32383: new effect 2, aura 108, misc 15, BasePoints 13/27/41, `EffectSpellClassMaskB_1`
  1. Sim `shadowbolt.go` crit multiplier secondary +0.14×rank. Alternative: Pandemic also covers Curse of
  Agony, 85.8–94.4%, misses the 86–94 design band.
- **Demonology.** 63117/63121/63123: new effect 2, aura 108, misc 15, BasePoints 9/19/29,
  `EffectSpellClassMaskB_1` 576 → 1 (stale mask), `B_2` 192. Sim `shadowbolt.go`, `incinerate.go`,
  `soul_fire.go` +0.1×rank. Best lever's growth with gear 1.62×, 1.72× needed; Felguard crit inheritance
  (P1/300 96.3) and Corruption crit (two hosts) rejected.
- **Destruction.** 18093/63243/63244 `EffectBasePoints_1` 1/3/5 → 6/12/19. Sim `talents.go`
  `pyroclasmDamageBonus`. Best growth 1.44×, 2.1× needed at P4; crit damage (P1/300 95.7) and Immolate haste
  ticks rejected.
- **Elemental.** 16041: new effect 2, aura 108, misc 15, BasePoints 109, `EffectSpellClassMaskB_1` 3 (the
  overloads 45284/45297 carry the same flags). Sim `electric_spell.go` `ElementalCritMultiplier(1.1)`.
  Rejected: Elemental Oath 30% (P1–P3 above 94).
- **Enhancement.** 29082/29084/29086 `EffectBasePoints_1` 3/6/9 → 11/23/35 (aura 79). Sim `talents.go` table.
  Rejected: Flurry 50% (P4/P5 miss).
- **Shadow.** 51167 `EffectBasePoints_1` 19 → 52 (aura 174). Sim `sim/priest/talents.go` factor. P3/180 85.8
  and P4/300 94.0 sit just outside 86–94; no lever measured fits both. Rejected: DoT base points (no shape under
  the DoT haste tweak), Shadowform Mind Blast flat +2200 (breaks if the 1 s Mind Blast gate changes).
- **Feral.** Primal Gore 63503 `EffectSpellClassMaskA_1` 8388608 → 8392704 (adds Rake). Rend and Tear
  48432/48433/48434/51268/51269 `EffectBasePoints_2` 4/9/14/19/24 → 9/19/29/39/49. Idol 60773
  `EffectBasePoints_1` 89 → 134 (any larger flips P4's BiS idol). Sim `rake.go`, `talents.go`,
  `ferocious_bite.go`, `shred.go`. Rejected: flat ArP rating on Primal Gore (new mechanic).
