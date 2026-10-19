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
Warlock DM3/DM4/DM7, Priest S3, Shaman, Druid, Rogue, Warrior). `b12b91fae` (not deployed, needs a rebuild): the
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
| BM Hunter | 67.9 | 73.5 | Steady Shot 77 against 91; pet setup (above) the likely rest. Flagged |
| Demonology | 72.1 | 82.0 | no Curse of Doom (fixed in `b12b91fae`); Shadow Bolt 54 against 67.5. Flagged |
| MM Hunter | 84.1 | 87.3 | |
| Elemental | 83.5 | 87.7 | |
| Shadow | 73.3 | 89.4 | Mind Flay 65 against 82 |
| Combat | 89.6 | 89.6 | |
| Survival | 83.7 | 90.3 | |
| Fury | 95.0 | 90.5 | dances now (Rend up 66%, sim 77%); P2 didn't. Within noise, the void run had 92.3 |
| Balance | 83.9 | 91.4 | |
| Arms | 89.4 | 101.9 | Slams as the sim does |

Damage by source (`botdps compare -damage`; pet stats from the setup's pet `info` line, `tmp/p3/botdps-pets/`):
- BM: the pet's melee is 2900 of the 3930 DPS missing (bot 3974, sim 6868); the hunter is close (6165, 6847).
  Swings 344 against 412, mean 2079 against 2999, crit 18-24% against 32% (glancing 27%, the sheet 35%). Same
  pet abilities cast, every passive talent on (Cobra Reflexes, Spiked Collar, Spider's Bite, Shark Attack, Wild
  Hunt), pet AP 4008 where the sim's inheritance gives about 4025. No module hooks the melee roll. Unchecked in
  the sim: the fight replays fight without pets (`fight.go`). Next: a pet melee parity probe; BM is the ranking's
  top, the tiers' reference.
- Combat: white swings 14% weaker (1534 against 1792) at the same crit and hit count, Sinister Strike 9%; the
  weapons are the setup's. The compare doesn't split the hands yet.
- Demonology, a third run 85.0%: Shadow Bolt 42 and Incinerate 40 against 67.5 and 26.9 (Molten Core charges),
  12 fewer fillers in all.

Next: rebuild for `b12b91fae`, re-measure Demonology; the BM pet probe.
