# Spec balance: P4 proposal

One server tweak per build still below B at the baseline (PLAN, "In-scope builds and baseline"), for the user's
review in one sitting; only approved tweaks go to P5. Each was prototyped in a per-class copy of the sim and
measured with the rankings runner (P1–P5, 180/300 s, the baseline's captures, seed 424242, 10k iterations), then
all together in one copy: `tmp/p4/combined/` (only the 15 targeted builds move; every other build is identical
to the digit). Prototype diffs: `tmp/p4/patches/<class>.patch` (gitignored; values hard-coded, P5 redoes them
per PLAN P5). No WotLK Classic tuning applies to any of these builds (searched per class; Classic only tuned
Ret, Feral's Omen and Demonic Pact, all already in the module or the sim), except SoD's Explosive Shot base buff
(precedent for Survival's tweak).

## Decisions for the user

1. **New effects on existing talents.** The PLAN allows "no new spells or mechanics". Value-only levers were
   measured first and fail these builds, so seven tweaks add an effect of an aura type the server already
   handles to a talent's free effect slot (SQL only, no C++): Arms, Fury (armor pen, aura 280), MM (ignore armor,
   aura 108 op 13, which no 3.3.5 spell uses), Elemental, Affliction, Demonology (crit damage bonus, aura 108
   op 15), Ret (a proc copying T10 2P, a flat Consecration mod). Affliction and Demonology use hosts unrelated to
   the spell they buff (Empowered Corruption, Nemesis), picked as spec-only, passive and with a free slot.
   *Recommended: accept.*
2. **Re-optimization.** Arms, Fury and MM get armor levers; with them, ArP gems lose to Str/Agi gems at P3–P5.
   After the next BiS chain or P6's stat-weight regen (bots gear by those weights), estimated: Arms P3–P5/300 at
   98–99, Fury 2–4% over BM at P4/P5, MM P3/300 at 96.4. *Recommended: before P5 lands them, rerun the BiS
   optimizer for these three builds with the tweak in the sim and re-size against those presets
   (MM's fallback is −13%/rank).* SV (checked: gems don't flip), Feral (idol checked) and Shadow (+0.4% at most)
   hold; the crit, haste and spirit levers of the others shift weights less and were not re-geared.
3. **Fury** takes the P4/300 top from BM by 0.2% (its own cell +0.13); no lever lifting P1/180 by 6% avoids
   it, and decision 2 makes it worse. *Recommended: no Fury tweak; its P1/P2 cells stay 80–83.*
4. **Fire** can't reach B under the cap: P4/P5/300 are ≥ 95 and allow +0.3, while every lever measured gives P2
   at most 1.5× P5's gain (30× needed). The closest (Living Bomb +12%) lifts each cell 0.3–0.4.
   *Recommended: no Fire tweak* (alternatives: Living Bomb +12%; or waive Fire's cap, Living Bomb ×2:
   P1–P3 83–87, P4/P5 96–99.7).
5. **Ret, Demonology, Destruction:** closest results, cells missing as listed below. *Recommended: accept; rerun
   Destruction's P4 BiS first (setup finding below, about +3 points at P4).* Ret's Consecration roughly
   quadruples per tick, an AoE buff on trash too.
6. **Feral's idol** is an item change, not a talent gate (Shred is cat-only, so only Feral gains). Without it,
   P1/180 is 84.96. *Recommended: accept.*
7. **Cap-exempt fixes** (below): *recommended: apply in P5.*

## Predicted % of top, all tweaks together

Recommended set minus Fury and Fire (decisions 3, 4) changes only the Fury rows and the P4/P5 300 s tops:
without Fury, BM keeps P4/300 and every P4/P5 300 s figure below rises 0.1–0.2.

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

## Tweaks

All SQL: full-row `spell_dbc` overrides through [tweaks] `tools/dbc_export.py --generate-override`, static, no
config switch, server-only fields (tooltips go stale). Effect slots are 1-based as in the generator's columns;
"new effect N" means `Effect_N=6` (apply aura), `EffectDieSides_N=1`, `ImplicitTargetA_N=1` plus the listed
fields. Each server path was checked in the AzerothCore source; the sim model named matches it.

**Arms:** Endless Rage ignores 36% of armor; Wrecking Crew's enrage 2%→~4.8% a rank.
- 29623: new effect 2, aura 280 (armor pen %), BasePoints 35; class mask B words stay 0. Sums with rating and
  Battle Stance under the ArP cap (`Unit::CalcArmorReducedDamage`).
- Wrecking Crew enrage 57518–57522 `EffectBasePoints_1` 1/3/5/7/9 → 4/9/13/18/23 (aura 79 physical).
- Sim `sim/warrior/talents.go`: `BonusArmorPenPct` +36 with Endless Rage; enrage table {.05,.10,.14,.19,.24}.
- Rejected: Sudden Death 20% (+1%), Endless Rage 75% rage (+1.8%), Wrecking Crew alone (P1/P2 miss).

**Fury** (not recommended, decision 3): Unending Fury ignores 3/7/10/14/17% armor: 56927/56929–56932 new
effect 1 aura 280, BasePoints 2/6/9/13/16, `EffectSpellClassMaskA_1` 33554432→0 (a stale Mortal Strike mask on
the empty slot that would limit it to MS).

**Retribution** (closest): (1) Improved Judgements 25956/25957 gets T10 2P's Divine Storm reset, off while T10 2P
is worn: `ProcTypeMask` 0→4, `ProcChance` 101→20/40, new effect 2 aura 4 (dummy) `EffectTriggerSpell_2` 70769;
`spell_proc` row -25956 (ProcFlags 4, DisableEffectsMask 1, rest 0); `conditions` rows source 24, entries
25956/25957, type 1 (aura) 70765, negated. (2) Swift Retribution 53379/53484/53648 new effect 2: aura 107
(flat mod), misc 3 (SPELLMOD_EFFECT1), BasePoints 399/799/1199, `EffectSpellClassMaskB_1` 0→32 (Consecration).
- Sim: `sim/paladin/talents.go` `applyDivineStormReset`, `sim/paladin/consecration.go` +400/rank.
- P5: confirm in game that slot 2 works beside `spell_pal_swift_retribution` (hooks the first aura 107) and the
  condition blocks the reset under T10 2P.
- Rejected: Holy Vengeance flat (worse shape); Exorcism without cooldown on Art of War +4600 (83–84 early, but a
  cooldown change, always crits undead/demons, and under the bots' priority P4/300 ≈ 95.8).

**Combat:** Lightning Reflexes melee haste 4/7/10% → 9/16/23%: 13712/13788/13789 `EffectBasePoints_2` 3/6/9 →
8/15/22 (aura 138), generated with `--dbc-path A:/WOW/dbc/Changed`. Rupture gains ticks through the module's
haste tweak. Sim `sim/rogue/talents.go` table {1, 1.09, 1.16, 1.23}. Rejected: Savage Combat AP, Combat Potency
40% (energy, depends on bot spending).

**Marksmanship:** Marked for Death's shots ignore 15% of armor a rank, before ArP; nothing once ArP is capped.
53241/53243–53246 new effect 3: aura 108, misc 13 (SPELLMOD_IGNORE_ARMOR), BasePoints −16/−31/−46/−61/−76,
`EffectSpellClassMaskC_1` 397313 (Auto, Multi, Aimed, Silencing), `C_2` 8388609 (Steady, Kill Shot). Pet hits and
spells unaffected; also applies against players. Sim: new `Spell.IgnoreArmorPct` (`sim/core/spell.go`,
applied in `spell_resistances.go` `GetArmorDamageModifier`), set in `sim/hunter/talents.go`. Rejected: +4%/rank
armor pen (P3/300 95.8), flat Steady Shot (no shape).

**Survival:** Explosive Shot base per tick ×2, AP part unchanged (SoD's lever: +15% then +50%).
`EffectBasePoints_1/DieSides_1`: 53301 143/29→287/59, 60051 220/45→441/89, 60052 324/67→649/133, 60053
385/79→771/157. Sim: serverdata 60052/60053, `sim/hunter/explosive_shot.go` tick ranges. SoD's exact ×1.725 gives
P2/300 85.1.

**Arcane:** Arcane Empowerment's SP coefficient bonus to Arcane Blast and Missiles 3/6/9% → 5/10/15%: 31579/
31582/31583 `EffectBasePoints_1` 2/5/8 → 4/9/14. Sim `arcane_blast.go`, `arcane_missiles.go` Flat 5×rank.
Rejected: Spell Power crit damage (P2/300 85.2).

**Fire** (not recommended, decision 4): Living Bomb 55360 `EffectBasePoints_1` 344→385 (keep
`EffectBasePoints_2` 55361), explosion 55362 689→771.

**Affliction:** Empowered Corruption adds 14% Shadow Bolt crit damage bonus a point (crit ×2.09 → ×2.32 at 3/3):
32381–32383 new effect 2, aura 108 misc 15, BasePoints 13/27/41, `EffectSpellClassMaskB_1` 1. Sim
`shadowbolt.go` crit multiplier secondary +0.14×rank. Alternative (pattern-conforming): Pandemic also covers
Curse of Agony, 85.8–94.4, misses the 86–94 design band.

**Demonology** (closest): Nemesis adds 10% crit damage bonus a point to Shadow Bolt, Incinerate, Soul Fire:
63117/63121/63123 new effect 2, aura 108 misc 15, BasePoints 9/19/29, `EffectSpellClassMaskB_1` 576→1 (stale
mask), `B_2` 192. Sim `shadowbolt.go`, `incinerate.go`, `soul_fire.go` +0.1×rank. Best lever ratio found 1.62,
1.72 needed; Felguard crit inheritance (P1/300 96.3) and Corruption crit (two hosts) rejected.

**Destruction** (closest): Pyroclasm 2/4/6% → 7/13/20%: 18093/63243/63244 `EffectBasePoints_1` 1/3/5 → 6/12/19.
Sim `talents.go` `pyroclasmDamageBonus`. Best ratio 1.44, 2.1 needed at P4; crit damage (P1/300 95.7) and Immolate
haste ticks rejected.

**Elemental:** Call of Thunder adds 110% crit damage bonus to Lightning Bolt and Chain Lightning, overloads
included (crit ×2.09 → ×2.69): 16041 new effect 2, aura 108 misc 15, BasePoints 109, `EffectSpellClassMaskB_1`
3. Sim `electric_spell.go` `ElementalCritMultiplier(1.1)`. Caveat: the server's overload takes half the bolt's
dealt damage (a crit halves rather than divides by the multiplier), the sim rolls its own half bolt (documented
gap): live may run +1.1–1.8% above the sim, P2/P3 near 94.5. Fix the gap first and re-size to about 1.0.
Rejected: Elemental Oath 30% (P1–P3 above 94).

**Enhancement:** Weapon Mastery 4/7/10% → 12/24/36% physical: 29082/29084/29086 `EffectBasePoints_1` 3/6/9 →
11/23/35 (aura 79). Sim `talents.go` table. Rejected: Flurry 50% (P4/P5 miss).

**Shadow:** Twisted Faith 5/5 spirit to spell power 20% → 53%: 51167 `EffectBasePoints_1` 19→52 (aura 174).
Sim `sim/priest/talents.go` factor. P3/180 85.8 and P4/300 94.0 sit just outside 86–94; no lever measured fits
both. Rejected: DoT base points (no shape under the DoT haste tweak), Shadowform Mind Blast flat (+2200; breaks
if the 1 s Mind Blast gate changes).

**Feral:** (A) Primal Gore lets Rake ticks crit as T10 4P does: 63503 `EffectSpellClassMaskA_1` 8388608→8392704.
(B) Rend and Tear's Bite crit 10% a rank: 48432/48433/48434/51268/51269 `EffectBasePoints_2` 4/9/14/19/24 →
9/19/29/39/49. (C) Idol of the Ravenous Beast Shred +90 → +135: 60773 `EffectBasePoints_1` 89→134 (any larger
flips P4's BiS idol). All three do nothing at P4/P5 gear. Sim `rake.go`, `talents.go`, `ferocious_bite.go`,
`shred.go`. Rejected: flat ArP rating on Primal Gore (new mechanic).

## Cap-exempt findings

- Combat: Lethality 5/5 instead of Vigor (Assassination tree `00532000514`): +0.16–0.41% in all cells; preset,
  presetgen and bots.
- Enhancement: Glyph of Stormstrike instead of Flametongue Weapon (major 2): +0.3–0.5%; `ui/enhancement_shaman/presets.ts`.
- Ret, once its tweak is in: Consecration first in the APL and the bots' strategy, +3.7% P1, +0.7% P4 (P4/300
  ≈ 94.6).
- Destruction's P4 BiS wears 2P T9 + 2P T10; a 4P T10 swap gains +4.45%: rerun its BiS.
- Shadow's P5 BiS sims below its P4 BiS (17873 against 17958): look in the BiS chain.
- Enhancement: Shamanistic Rage on cooldown with T10 2P is unmeasured; the BiS skips T10 at P4/P5.

## Parity gaps found (not P4)

- Lightning Reflexes ranks 1/2: the sim has 3/6% haste, the server 4/7%.
- Demonic Pact: the sim's ICD is 1 s, the server's `spell_proc` 54909/53646 has 20 s since world update
  2026_03_16_00 (Demonology −0.03 to −0.21%).
- Lightning Overload's damage (Elemental, above).
