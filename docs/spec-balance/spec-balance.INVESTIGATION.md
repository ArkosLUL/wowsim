# Spec balance: P1 investigation

Sim rotation, talent, glyph and spec-option audit of the 16 builds below B (PLAN, P1). Every DPS figure: `apldbg`
on the rankings' 2026-10-18 captures standardized as the rankings run them (Playerbot switch on), seed 101,
10k–20k iterations; every applied fix wins beyond 2 se in all ten cells (P1–P5 × 180/300 s) and was reproduced by
the review. Gains below are P1/180, P1/300, P5/180, P5/300.

## Applied fixes

| Build | What was wrong | Fix | Gain % |
|---|---|---|---|
| Retribution | Exorcism above Consecration; HoW and Judgement above CS cost strike GCDs | `default.apl.json`: CS > J > DS > HoW > Consecration (> 4 s left) > Exorcism (AoW) | +0.15, +0.50, +0.59, +0.67 |
| Arms | Recklessness, now an in-combat cooldown, leaves Berserker Stance on; only Shattering Throw's own swap returned it (Throw off: −19%) | `arms.apl.json`, `arms_sunder.apl.json`: cast Battle Stance after `autocastOtherCooldowns` | ranked ±0; Throw off +25.6 (P1), +23.8 (P5) |
| Fury | Preset spent 3 points on Deflection and Improved Cleave while the APL stance-dances for Rend/Overpower | `FuryTalents` = the TestFury suite's: Improved Rend 2, Improved Overpower 1, Taste for Blood 1 | +0.71, +0.70, +0.76, +0.86 |
| Enhancement | Fire Elemental Totem never cast (option off); in-combat Call of the Elements dismissed it | `useFireElemental` on; the three APLs cast CotE only while the elemental is down, then the elemental | +8.50, +5.14, +4.91, +3.04 |
| Elemental | Fire Elemental sat in a sequence behind the potion, which the switch drinks at 5 s, so it never fired | `default.apl.json`: elemental while Wild Magic is up or from 10 s; Blood Fury alone; Searing waits for it; on-use trinkets and a switch-off potion line kept | +10.76, +6.47, +6.69, +4.26 |
| Combat | autoRotation always chose Expose Armor, wasted under the raid's Sunder | `ui/rogue/sim.ts`: plain Combat when the raid has Sunder or Expose Armor | +5.97, +5.57, +4.06, +3.82 |
| Feral | First major glyph slot empty | Glyph of Rip | +1.32, +1.36, +0.90, +0.96 |
| Marksmanship | Cooldowns gated to 10 s, missing Bloodlust; dead talent points (TSA, Focused Aim past hit cap) | APL gate removed; FA 1, TSA 0, Improved Steady Shot 2 | +1.43, +1.39, +1.17, +1.00 |
| Survival | Same 10 s cooldown gate | Gate removed | +0.69, +0.52, +0.32, +0.35 |
| Fire | Blast Wave and Dragon's Breath never cast; Student of the Mind 1/3 | Both dropped, Student 3/3; minor glyph Blast Wave → Arcane Intellect | +0.47, +0.47, +0.36, +0.54 |
| Arcane | Page kept Fire's consumes (PLAN setup bug) | `ui/mage/sim.ts` swaps untouched default consumes on a Fire/Arcane tree change | 0 (switch sets consumes) |
| Demonology | Sims with Affliction's Felhunter and Spellstone (PLAN setup bug); Intensity 2 instead of Aftermath 2 | `ui/warlock/sim.ts` sets the tree's preset options on a tree change unless hand-picked; Aftermath 2 | +14.4, +14.5, +12.0, +12.1 |
| Destruction | Same setup bug (no Imp, no Empowered Imp) | Same hook (Imp + Firestone) | +6.8, +8.1, +4.0, +4.6 |
| Shadow | Mind Blast hidden, never cast | Unhidden while its cast time ≥ 1 s (past 50% haste it stops paying) | +2.0, +2.2, +0.04, +0.10 |
| Affliction | Potion led the end-of-fight sequence, so it blocked under the switch; no `autocastOtherCooldowns` | Autocast added; potion moved last | +1.2, +0.7, +0.9, +0.7 |

The warlock, mage and rogue `sim.ts` changes act at capture time: the rankings show them only after a recapture.
Goldens promoted: TestRetribution, TestArms, TestElemental, TestEnhancement, TestMM, TestSV, TestShadow,
TestAffliction; every other suite unchanged.

Frost DK: no fix inside P1's paths. Arcane and Arms are rotation-clean; their gaps are P4 work.

## Decisions (user, 2026-10-19)

- **Frost DK ranks 'Frost UH'** (existing preset, `frost_uh_pesti.apl.json`; `tools/presetgen/gen.py`):
  +10.2/+10.2/+11.1/+11.1%, as it takes Epidemic, which carries the module's disease-haste tweak. If P4 limits
  that tweak to Unholy, the gain reverses. The user's Frost DK bots need these talents too.
- **Fire keeps TTW.** Frostfire (0/56/15) would add +4.0/+4.1/+2.0/+3.7%; declined.
- **Bot food**, applied to `Playerbot.env`'s matrix (step 4, `tmp/p1/consumables/proposals.md`; the bot's flask is
  already best for all 20 builds):
  - Fury → Dragonfin Filet (+0.13 to +0.75%);
  - Combat → Mega Mammoth Meal (+0.30 to +0.49%);
  - Frost DK → Dragonfin Filet (+0.34 to +0.77%; row 73 covers Frost tanks too);
  - Enhancement → Fish Feast (+0.29 to +0.46%);
  - Arcane → Imperial Manta Steak (+0.04 to +0.15%).
- **Potion of Speed for Shadow, Balance and Affliction** (+0.03 to +0.64%): [bots] `GetOffensivePotionLadder`
  drives stocking and the drink lookup; the sim's `playerbotPotion` matches. Affliction rechecked with a bot-like
  rotation (Corruption only when missing): Speed still wins +0.36/+0.24/+0.36/+0.21%.
- **Combat's Glyph of Tricks of the Trade** stays (a self glyph: +0.5–0.7% to the rogue at the raid's cost).
- **UI tests** for the warlock and mage tree hooks added in `tools/uitest/tests/settings/talents.spec.ts`.

## Notes for later phases

- **BiS chain:**
  - rerun Demonology and Destruction (gear was optimized on a Felhunter);
  - rerun MM (gear carries 1–2% spare hit);
  - rerun Fire (new talents).
- **P2/P3, bot rotations to copy:**
  - Ret: CS > J > DS > HoW > Cons > Exo.
  - Fury: no stance dance; the new talents need Battle Stance → Rend/Overpower → Berserker.
  - Frost DK: bots cast Army of the Dead in combat (−1 to −2.5%).
  - Enhancement: no CotE or fire totem while the elemental is out; bots cast Fire Nova only in the AoE strategy.
  - Elemental: cast the elemental with the potion up.
  - Feral: Faerie Fire (Feral) on cooldown outside Clearcasting.
  - MM: bots weave Arcane Shot (−0.2 to −2.7%).
  - Hunters: cooldowns at the pull.
  - Warlocks: pet and stone per spec (Demo Felguard + Spellstone, Destro Imp + Firestone, Aff Felhunter + Spellstone).
  - Shadow: Mind Blast only while its cast is ≥ 1 s.
  - Affliction: recast Corruption when a damage or crit buff is up that the running Corruption missed (the sim's
    `warlockShouldRefreshCorruption`): +0.2/+0.8/+1.6/+2.5% over casting it only when missing. Everlasting
    Affliction refreshes carry haste and spell power, not crit or multipliers (server and sim agree).
- **Sim/server gaps:**
  - `Shaman.Reset` (`sim/shaman/shaman.go`) applies Totem of Wrath and its glyph's SP every reset, even while the APL
    swaps in Searing; recheck Elemental against P2.
  - Fish Feast and Great Feast give no ranged AP on the server, but the sim adds it.
  - The server applies Mixology to elixirs only for known recipes; the sim gives flasks to every alchemist.
- **P4:** Frost UH's gain rides on Epidemic's tweak. The remaining gaps at 180 s vs BM:
  - Demo 89/80%, Destro 84/75%, Affliction 88/82%, Shadow 82/86% (P1/P5);
  - Arms 70/79%;
  - Ret ~68% at P1 (idle GCDs; Holy Wrath can't hit a Giant).
- **Cap watch:** Fury reaches ~96% (P5/180) and ~99% (P5/300); Frost UH would reach ~98% at P1/300. Both are
  cap-exempt.
- Golden gap: TestCombat still runs `combat_expose`, so the new Combat default has no golden.
- Assassination's autoRotation has the same Expose Armor waste. It's at or above B, so left alone.

## P2 spike (2026-10-19): passed

A harness character fights the Naxx dummy under bot AI. Test `TestBotSpike` in `tmp/p2/e2e/botspike_test.go`
(a copy of [simval]'s committed `e2e/`, untracked), run by `tmp/p2/run-spike.sh <label> [class] [seconds] [potion]`
under the server lock; captures in `tmp/p2/captures/`. Recipe:

1. `newBot`, gear and talents (the spike: `.playerbots bot initself=epic`), `.ip set 18`, potion via `.additem`.
2. `.gm off` (the GM flag is saved and comes back on the bot, and a GM never enters combat), teleport it off
   Naxx (a group member enters the leader's instance only from another map), save, log out.
3. Log a second character into the same account (`LoginBots` creates it), GM on, level 80.
4. `.playerbots bot add <name>` (reply `add: <name> - ok`; same-account bots allowed); the bot joins the master's
   group by itself.
5. Raid convert (raw `CMSG_GROUP_RAID_CONVERT` 0x28E): a non-GM enters a raid map only in a raid group. `.summon`
   can't help, it moves only GM sessions into an instance (`Instance.GMSummonPlayer = 0`).
6. Master `.go`es into Naxx, party chat `summon`: the bot teleports into the master's instance.
7. The master tanks: a Night Elf druid with Ferocity 5/5 (`CMSG_LEARN_TALENT` 795 rank 4) in Bear Form, which
   playerbots count as a tank. Check the learn by free talent points (field 0x3FC) dropping 5: after a restart
   `character_talent` lags the save by over 10 s. Spawn the dummy, `.gm off`, select it, `.simval victim`,
   party chat `attack`.
8. Cleanup: `.playerbots bot remove`, then the master's `deleteCharacters` takes both characters.

Burst gate: the dummy (`NullCreatureAI`) never has a victim, so `TankHasHeldBoss` never passed. The user chose a
tank victim: `.simval victim` ([simval] `SimValidationDummy.cpp`). Verified: with it, Icy Veins and Mirror Image
come ~11 s after `attack`; without it (control run), never.

Potion: `OffensivePotionTrigger` needs `bot->IsInCombat()`, and the bot was in GM mode (step 2), so it never
fired; the burst multiplier has no combat check, so cooldowns went out. With `.gm off` the bot drinks Wild Magic
5.7 s after `attack`, as raid bots do (botobs traces, ~8.4 s into a pull). The earlier spike DPS figures (Fire
6168, Assassination 6837) came from out-of-combat bots and don't count. `tools/simval chronicle` reads the
bot's logs; the bot had the matrix's flask and food and loaded its raid strategy.

Spike switches: `SIMVAL_BOTSPIKE_DEBUG` (party `co ?`, `nc +debug`: the bot whispers each action run),
`SIMVAL_BOTSPIKE_PROBE` (party lines at 20 s, `;;`-separated), `SIMVAL_BOTSPIKE_NOVICTIM`,
`SIMVAL_BOTSPIKE_GO` (the fight spot), `SIMVAL_BOTSPIKE_LOGLEVEL` (GM 4 on the test account, `playerbots` logger
at debug during the fight: `T:`/`PUSH:`/`A:` engine lines in `Playerbots.log`). Potions left are summed per
stack (`item_instance.count`).

P3 candidate: after drinking, the bot retries `offensive potion` every ~2.5 s, failing each time (raid trace
`649_2_thrakgar_1792361718`: 105 of 127 attempts FAILED). `PotionSpellsReady` (`UseItemAction.cpp`) is meant to
stop that but doesn't see the in-combat potion lock. No lost casts seen, wasted AI ticks only.
Also seen: a rogue bot applied its poisons after the pull.

## P2 results (2026-10-19)

`TestBotDps` ([simval] README) fought each build 180 s, five at a time; `tools/simval botdps compare` (its README
section) simmed the same. Phase P3: the core raid is on ToC 25H and Ulduar 25 (26 characters bound to maps 649 and
603, none to ICC; `character_instance`). Settings: the 20 builds captured at P3 from a dev server on [bal]
`72287986a`. Files in `tmp/p2/`: `capture/settings/`, `botdps_setups.json`, `botdps/` (one fight per build),
`botdps-repeat/`, `compare-v2.txt`, Fire's serial and parallel control in `botdps-serial/` and `botdps-control/`.
The first run (`botdps-v1/`) is void: its bots lacked the professions' passives the sim gives (Master of Anatomy,
Toughness, Mixology), and its window ended at the last hit Chronicle had flushed.

Noise: one 180 s fight varies about ±4.5% (1σ). Fire serial 103.9%, parallel 93.3%: the same casts (65 Fireballs
each), 29 against 23 Hot Streaks, so crit luck, not the parallel runs. Arcane swings most: 80.2% and 93.1%. Builds
near the flag line ran twice.

Bot/sim, the mean of two runs where repeated; median 89.0%, flag line 84.0%:

| Build | % | Main differences (bot against sim) |
|---|---|---|
| BM Hunter | 67.9 | pet Bite 94 against 116, Growl 48 (focus); Steady Shot 75 against 91 (the sim casts Aimed Shot 16.9 and Explosive Trap 6.0 too; the bot's traps count under 425782) |
| Demonology | 72.1 | 8 Life Taps (11.4), the last at 107 s, then wands from 155 s (18 shots): eating swapped Fel Armor for Demon Armor; Immolate up 80% (93); Curse of Agony, not Doom |
| Shadow | 73.3 | Mind Flay 64 channels (82) of 2.6 ticks; Mind Blast 23 against 14.6 (the sim's 9.6 under 63675 is Improved Devouring Plague; the bot cast 8 Plagues) |
| Elemental | 83.5 | Chain Lightning 32 and 29 on one target (0); Lightning Bolt 63 and 67 against 114.5 (overloads, 45284, not counted) |
| Survival | 83.7 | Explosive Trap 1 against 7.7 |
| Balance | 83.9 | Moonfire up 52% and 43% (97); Starfire 41 against 54 |
| MM Hunter | 84.1 | no Silencing Shot (10); Explosive Trap 1 against 7; pet Growl |
| Arcane | 86.7 | Arcane Blast 80 and 89 against 100; Frostbolt 30 (10) |
| Frost DK | 87.3 | Obliterate 37 against 44; Army of the Dead in combat |
| Unholy 2H | 88.6 | Scourge Strike 11 against 27; Death and Decay 6 (0) |
| Arms | 89.4 | never Slam (43.5); Heroic Strike 46 against 26 |
| Combat | 89.6 | Rupture up 77% (87); the sim's finishers count under combo point tags (Rupture 7.9 at 5 points, Eviscerate 1.7); hits 10-15% weaker than the sim's, unexplained (worth a sim against server stat check) |
| Affliction | 91.6 | |
| Fire | 92.8 | |
| Feral | 92.9 | Ferocious Bite 5 against 7.7 |
| Assassination | 93.5 | |
| Retribution | 94.6 | |
| Fury | 95.0 | never Rend or Overpower (9.6, 6.9): no stance dance |
| Enhancement | 96.6 | Fire Nova 18 against 38 |
| Destruction | 97.1 | |

P3 (PLAN trigger: more than 5 points below the median, a key spell never cast, or an uptime under 80%):
BM, Demonology, Shadow; Elemental, Survival and Balance (under the line by less than a point, the first and last
with clear mistakes too); MM (Silencing Shot); Arms and Fury (spells never cast); Combat (Rupture uptime).

What the test itself does, so a difference isn't the bot's:
- A Life Tap bot stops below `Playerbots.LowHealth` (45%) and wands; the tank Full Heals the bot below max health
  every 8 s, as raid healers would. It never lands: the server takes `.cast 25840 triggered` without an error,
  yet no bot log has a Full Heal, and a rogue (no regen in combat) gets one at every check. Open.
- The test's Blood Frenzy refresh knocks the rogue's Savage Combat off; its Thunder Clap pulls critters in.
- Buffs the server keeps off: Trueshot Aura and Unleashed Rage (Abomination's Might holds their slot), Elemental
  Oath (Moonkin Aura's), Curse of Weakness (misses). Left out: Devotion Aura, Sanctuary, Might for a caster or
  Wisdom otherwise, Expose Armor, Heroic Presence, Mana Tide, Replenishment, Demonic Pact, Thorns, Judgements of
  Light and Wisdom, Shadow Embrace. The sim goes without each.
- Hunter pets: `tame family Wolf`, pet talents the bot's own (`InitPetTalents`); the sim's BM has BM pet talents.
- Report quirks: Mirror Image's casts per image (three times the sim's), the Infernal's Immolation ticks (20153),
  procs as casts, some spells under other ids in the sim (Mutilate's hits, Envenom, Stormstrike's off hand,
  stances). Shadow Mastery's bot uptime is the bot's own share; the master keeps it up too.
- The sim's Rend bonus above 75% health follows time left; the dummy's health never drops, so Arms' sim is low.

## P3 results (2026-10-20)

[bots] `Custom` `f8e88b208..2586e5b25` (deployed): hunters use potions and trinkets while Auto Shot repeats; pet
taunts off autocast while the group has a tank; per-class fixes in `docs/classes/<class>.md` (Hunter D8-D10,
Warlock DM3/DM4/DM7, Priest S3, Shaman, Druid, Rogue, Warrior). `b12b91fae` (deployed by a second rebuild): the
curse strategies are siblings, so Demonology's `curse of doom` was evicted by the `curse of agony` listed after it
(0 Doom casts against 4.0); Doom now carries its own Agony fallback.

Test fix ([simval] `f63562a`): `playerbots_db_store` keeps a bot's strategies by guid after the character is
deleted, and a restart reissues those guids, so the first P3 run's BM, MM, SV and Arms bots fought with other
classes' strategies (no damage) and Balance with only its DoTs (void, `tmp/p3/botdps/`). The test now sends
`reset botAI` once the bot is in Naxxramas. P2 may have hit the same with a same-class row; its bots cast their
spec's spells.

Bot/sim, one 180 s run (`tmp/p3/botdps-v2/`, `compare-v2.txt`); median 89.5%, flag line 84.5%:

| Build | P2 | P3 | Left (bot against sim) |
|---|---|---|---|
| BM Hunter | 67.9 | 73.5 | pet without its talents and raid buffs; fixed, 87.2 then 92.8 (below) |
| Demonology | 72.1 | 82.0 | no Curse of Doom; fixed, four runs 89.2, 85.6, 90.4, 83.6 (below) |
| MM Hunter | 84.1 | 87.3 | |
| Elemental | 83.5 | 87.7 | |
| Shadow | 73.3 | 89.4 | Mind Flay 65 against 82 |
| Combat | 89.6 | 89.6 | 98.8 without Prey on the Weak (below) |
| Survival | 83.7 | 90.3 | |
| Fury | 95.0 | 90.5 | dances now (Rend up 66%, sim 77%); P2 didn't. Within noise, the void run had 92.3 |
| Balance | 83.9 | 91.4 | |
| Arms | 89.4 | 101.9 | Slams as the sim does |

Damage by source (`botdps compare -damage`; pet stats from the setup's pet `info` line, `tmp/p3/botdps-pets/`):
- BM: the pet's melee was 2900 of the 3930 DPS missing: swings 344 against 412, crit 18% against 32%, hits
  smaller. Two causes:
  - A freshly tamed pet casts its passives before `SetMinion` gives it an owner, so the hunter's spell mods on
    them (Ferocity, Unleashed Fury, Serpent's Swiftness, Bestial Discipline, ...) stay off until it's loaded from
    the DB again. Core Tame Beast does this for players too; [bots] `919092c27` recalculates the pet's passives in
    `PlayerbotFactory::InitPet` (`tame` runs it). BM 87.2%, pet crit and swings now the sim's
    (`tmp/p3/botdps-fix1/`).
  - The test put single target buffs and Kings on the bot only, so the pet had 10% less of every stat. [simval]
    `851f059` buffs the pet too: its stats match the sim's; BM 92.8% (`botdps-fix2/`).
- `.simval info`'s `critVsOther` against a pet adds (400 - the GM's current defense skill, 1) x 0.04 = 16%: the
  pet's "35%" sheet crit was 19%.
- Combat: every hit 8-14% weaker, Rupture and Instant Poison too, at the same stats (sim `ComputeStats` against
  `.simval info`: AP 8241 against 8148, crit, hit, expertise equal). Prey on the Weak: the sim has it up 150 s (its
  target's health drains with time left), the dummy never leaves 100%. Without the talent the sim does 11480, bot/sim
  98.8%. Execute ranges are off on both sides (`botdps compare` sets no execute proportions); Arms' Rend is the only
  other in-scope mechanic on the target's health.
- Demonology, a third run 85.0%: Shadow Bolt 42 and Incinerate 40 against 67.5 and 26.9 (Molten Core charges),
  12 fewer fillers in all.
- Demonology after `b12b91fae` (`tmp/p3/botdps-demo/`, `botdps-demo2/`): 89.2% and 85.6%, unflagged. Curse of
  Doom hits 4 times a run like the sim, hasted (32 s under Bloodlust, 42 s after) and critting (0/4, then 3/4).
  Immolation Aura 0 against 2 (330 DPS): the bot casts it only in its AoE strategy within 5 yd; the sim assumes
  melee range. Shadow Mastery up 71-74% against 96%. Run 2: Molten Core procs stop after 80 s (3 against 11; 62
  Corruption ticks without one, 0.04% by chance at 12%), so Incinerate 6 against 26.9; no module touches it,
  unexplained. Two later runs proc normally (12, 10): 90.4% and 83.6% (crit 43% against 53%). Totem of Wrath
  "gone by the end" is Demonic Pact replacing it, as in the sim.
- Full Heal: `.cast 25840 triggered` gets no reply and the bot's health doesn't move. Harmless so far: warlock bots
  stay above 75% between checks.

Next: no build flagged. Open: Molten Core's one stall; Full Heal; Prey on the Weak against a full health dummy.

## Baseline (2026-10-21)

Rankings `3b28d98dc`, table in the PLAN. Per build: the lift its worst cell needs to reach 85%, the room its best
cell has below 95% (none: already ≥ 95%, so ±0.3 points there), and how many of the 10 counted cells are below B.

| Build | Lift | Room | < 85 | | Build | Lift | Room | < 85 |
|---|---|---|---|---|---|---|---|---|
| Retribution | 23.7% | 7.4% | 6 | | Fury | 6.0% | none | 4 |
| Arms | 21.6% | 16.3% | 10 | | Demonology | 5.9% | 3.4% | 7 |
| Destruction | 14.7% | 6.9% | 9 | | Combat | 5.1% | 12.3% | 10 |
| Elemental | 13.3% | 14.2% | 10 | | Arcane | 4.8% | 11.3% | 8 |
| Enhancement | 11.4% | 10.5% | 8 | | Survival | 4.8% | 7.3% | 3 |
| Fire | 9.7% | none | 6 | | Shadow | 3.8% | 3.4% | 3 |
| Marksmanship | 7.7% | 2.6% | 4 | | Affliction | 1.5% | 2.5% | 4 |
| Feral | 6.2% | 0.1% | 5 | | | | | |

Weak early, strong late (a gear-shaped lever must help low gear only): Ret, Arms, Fire, Fury, Feral, MM, SV.
Weak late: Destruction, Elemental, Enhancement, Demonology, Affliction. Flat: Combat, Arcane, Shadow.
