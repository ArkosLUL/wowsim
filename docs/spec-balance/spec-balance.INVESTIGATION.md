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
2. Teleport it off Naxx (a group member enters the leader's instance only from another map), save, log out.
3. Log a second character into the same account (`LoginBots` creates it), GM on, level 80.
4. `.playerbots bot add <name>` (reply `add: <name> - ok`; same-account bots allowed); the bot joins the master's
   group by itself.
5. Raid convert (raw `CMSG_GROUP_RAID_CONVERT` 0x28E): a non-GM enters a raid map only in a raid group. `.summon`
   can't help, it moves only GM sessions into an instance (`Instance.GMSummonPlayer = 0`).
6. Master `.go`es into Naxx, party chat `summon`: the bot teleports into the master's instance.
7. Spawn the dummy, party chat `co -burst`, select the dummy, party chat `attack`.
8. Cleanup: `.playerbots bot remove`, then the master's `deleteCharacters` takes both characters.

Burst gate: the dummy (`NullCreatureAI`) never has a victim, so `TankHasHeldBoss` never passes. `co -burst` frees
the cooldowns, but `OffensivePotionTrigger` checks the tank hold itself, so the potion never fires. Ways out:
leave the potion out of both the bot run and its sim run; add 999000 to [bots]' `noVictimBosses` (cooldowns and
potion at the pull; rebuild); or a `.simval` command giving the dummy a tank victim (5 s gate as in a raid; C++ in
[simval], rebuild).

Results, 60 s, factory gear: Fire mage 6168 DPS, Living Bomb and Improved Scorch ≥ 96% uptime, Combustion and
Mirror Image cast; Assassination rogue 6837 DPS at 1.2 yd. Both had the matrix's flask and food, and the bot
loaded its Naxx strategy. `tools/simval chronicle` reads both logs. The rogue poisoned its weapons in combat.
