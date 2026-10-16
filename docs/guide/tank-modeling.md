# Tank modeling

What the BiS optimizer's tank runs fight and how they score (`ui/core/optimizer/pool_builder.ts`,
`sim/encounters/optimizer_tank_boss.go`, `sim/optimizer/{objective,caps,api}.go`). The tank suites' goldens use
the melee-only `GenericBossTarget` instead, with Patchwerk's numbers.

## The boss

`TANK_BOSSES` holds one boss per content phase; `TestTankBossesMirrorUI` keeps the Go mirror equal.

| Phase | Boss | Damage avoidance can't remove |
|---|---|---|
| 1 | Kel'Thuzad 25 | Frostbolt 55802 (half land), Frostbolt volley 55807 |
| 2 | Algalon 25, `dualWield`, `noParryHaste` | Quantum Strike 64592 (Physical), Phase Punch 64412, Black Hole Explosion 65108 |
| 3 | Anub'arak 25H, `noParryHaste` | Freezing Slash 66012, Penetrating Cold 68510 |
| 4 | The Lich King 25H, `noParryHaste`, `suppressDodge` | Soul Reaper 73799 with its follow-up, Infest 73781 |
| 5 | Halion 25H, `hitChance` 5, `enemyDodgeMultiplier` 0.8 | Flame Breath 74528 |

- The target is `OptimizerTankBossID`, a preset with an AI and no preset encounter. It swings from the boss's own
  `creature_classlevelstats` row (`damageExp2`, `attackPower`) times `damageModifier`.
- `dualWield` adds an off hand without the dual wield miss penalty: Algalon's Dual Wield 42459 gives 20% hit
  beside it.
- `noParryHaste` is the row's `CREATURE_FLAG_EXTRA_NO_PARRY_HASTEN`.
- `hitChance` and `enemyDodgeMultiplier` are Halion's Twilight Precision 78243. The multiplier is
  `Target.enemy_dodge_multiplier`, which core's melee table reads. Core has no hit for an enemy's swing, so the
  AI takes `hitChance` off `PseudoStats.IncreasedMissChance`.
- The boss's hit chance and each ability travel as labelled `target_inputs`, an ability starting at its
  "Spell id". The AI reads them by label: a missing one is 0, an unknown one panics.
- The AI casts an ability at a period rolled between its two. It never dodges, parries or blocks, so only the
  school's resistance and damage-taken multipliers cut it, and armor a `Physical` one (`DmgClass NONE` on the
  server; `SPELL_ATTR3_NO_AVOIDANCE` is never read).
- `canMiss` abilities roll `OutcomeMagicHit`, 1% for a level 83 creature against a level 80 player
  (`MagicSpellHitResult`); the rest use `OutcomeAlwaysHit`. By `DmgClass` and `Effect`s (`SpellHitResult`; no
  spell family or `SPELL_ATTR3_ALWAYS_HIT` on any of them): MAGIC rolls (Frostbolt, volley, Black Hole
  Explosion, Penetrating Cold, Flame Breath); NONE rolls only when every effect is school damage or a periodic
  damage aura (Quantum Strike, Infest). Phase Punch (periodic dummy), Freezing Slash and Soul Reaper (weapon
  percent damage) never miss.
- `followUpSeconds` and `followUpDamage` are a periodic aura's one tick after the cast, a DoT of it.
  `followUpHaste` multiplies the boss's melee speed for `followUpHasteSeconds` as it ticks (Soul Reaper's 69410).
- `swapsTank` (Soul Reaper, instant casts only): the other tank taunts after each cast, so the boss alternates.
  The modelled tank holds him first and takes the 1st, 3rd... cast, with its follow-up tick even after the
  boss left. Between casts 1 and 2 the boss neither swings at it nor casts at its victim; cast 2 lands on
  the other tank, who has just taunted back: the modelled tank holds the boss until cast 3, and takes the
  haste of cast 2's follow-up (a timer at the follow-up delay hastes the boss all the same). A `raidwide`
  ability (Infest, cast on the boss itself) still lands every cast, without restarting a swing. No taunt delay:
  Soul Reaper resets his swing, so a taunt inside the swing time (1.5 s) costs the tank nothing. The bots
  don't swap, and Algalon (traded at 4 Phase Punch stacks) isn't swapped, nor is Icecrown's 30% buff
  modelled: the user's calls. The Lich King's script has no Soul Reaper in phase 1 and 40 s into phases 2
  and 3; the fight casts it from 30.5 s.
- A cast with a cast time holds the boss's melee. A cast that fails its land chance is skipped and holds nothing:
  the raid interrupted it or it went to another player.
- `resetsSwing` restarts the boss's swing as the cast goes off (`Spell::IsAutoActionResetSpell`: untriggered,
  with `SPELL_INTERRUPT_FLAG_INTERRUPT`): Frostbolt, Freezing Slash, Soul Reaper, Infest, Flame Breath. After a
  cast time the swing comes a map update short
  ([spell data and timing](spell-data-and-timing.md#update-timing)).
- Kel'Thuzad's `landChance` 0.5 on Frostbolt is the share the raid doesn't interrupt (the user's call). His
  first 228 s, where he doesn't attack, and his adds are left out.
- Server data only: an ability goes in when the boss script and Spell.dbc give it a fixed number on the current
  tank. The server strips Physical from a mixed-school spell, so Freezing Slash and Soul Reaper are pure magic.
- Not modelled: abilities that follow the tank's health (Frost Blast) or position, and Big Bang, which hits
  whoever stays out of a black hole and stops Algalon's melee for 11 s.

## Healing

`healingModelForBoss` leaves `hps` 0, so `pinHealingModel` sets it once per run to 1.5 × the DTPS of the gear
as equipped (not the trimmed seed, which a small pool leaves nearly naked). Cadence is 1.5 boss swings, half
that under `dualWield`, varied by `TANK_HEAL_CADENCE_VARIATION` (0.5) of itself, so the tank can die.

Survival cooldowns fire below `hpPercentForDefensives`, 0.35 for a tank (`applySharedDefaults`); at 0 they
fire on cooldown (`major_cooldown.go`). A saved raider keeps what it was saved with: the request's
`cooldowns` comes from `rotation.simple.cooldowns`, so a save without it sends 0.

## Score

- `tankMetricWeights` and `TankWeights` give threat its slider share and split survival s: health margin s/2,
  DTPS, TMI and chance of death s/6 each (0.7 gives 0.35 and 0.1167). The Advanced editor overrides them.
- Armor is the reference stat for the four (`armorReferenced`), with a `normalizerOffset` of 2000. DTPS, TMI and
  the margin are dropped with a warning where their normalizer doesn't clear 2 se, the margin also where all its
  values are 0 (no healing model); death chance's rules are below.
- **Health margin**: each iteration's lowest health in the healed fight, read from core's passive shadow
  (`currentHealth - healthDebt`, `sim/core/health.go`: overkill adds to the debt, overheal pays it off), so it
  runs on past 0 and is ≤ 0 exactly where the tank died. Higher is better; no cap; a death save counts as
  survived. +X max health moves it by exactly X unless something scales with max health: below 35%, Ardent
  Defender's and Will of the Necropolis' cuts, the defensives' threshold and a death save's heal. Measured on
  the BiS fights, a Stamina point moves it 0.56 to 0.99 of the health it adds where the tank dies in up to 95%,
  1.2 to 1.6 where it dies in 99% and up. Core keeps it,
  and each first-death time, only for a unit that tanks with a healing model (`UnitMetrics` 19 to 22); the
  results page shows it beside TMI and chance of death.
- Chance of death pairs like the others: core lists each iteration's death (`UnitMetrics.iteration_deaths`,
  under `SaveAllValues`, for a unit that tanks). Where a pick all but never dies (Anub'arak), the death sixth
  is flat and only TMI and the margin price Stamina.
- **Death chance in two passes** (`optimize`). The normalizer must clear `deathMinSE` (10 se): worn gear
  that dies 10 to 30% of iterations clears it and prices Stamina sanely, while a gear that nearly always dies
  reads a few se and lets death chance swamp the score. The coefficient is capped at the Armor the loadout
  wears (J's points are Armor; a certain death can't cost more than the Armor). Where the worn gear fails the
  bar, the first search runs without the term and holds its warning back. Then the normalizer is measured at
  its pick (`measureDeath`, paired, ±2000 Armor): if it clears the bar, a second search warm-starts from the
  pick with death chance counted at that normalizer (`countDeathAt`), and its result stands, with a warning
  naming both; if the pick dies in 0 or 100%, or fails the bar, the first result stands with the warning of
  each. `TotalSims` counts what the passes simmed, not the second one's reads of cached points.
- Neither ±8000 Armor, 16,000 iterations nor healing at 0.75 to 4 times DTPS moves a worn gear that dies in
  every iteration off 100%, so whether a pick brings the term back is up to the pick.
- **Tiebreak** (`breakTie`, after verify's race): on a tank run whose best beats the seed, the loadouts within
  2 se of it that beat the seed too go to **time to live**: the same fight with healing off (Hps, cadence and
  variation 0, burst window kept), 60 s flat, a cloned request, seconds to the first death (60 alive). The
  longest mean goes first, and a warning names each one's time and score. Its sims count in `TotalSims`.
- **Gem check** (`checkGems`, every run, before the tiebreak): gems are priced off the curves and never simmed
  alone, so a small edge can fill every socket with one gem. For each distinct gem in the best's free non-meta
  sockets, a twin regems the slots holding it at the surrogate's slopes there with that gem banned
  (`gemWithout`), so the runner-up `gemOptions` drops takes its place with socket bonuses and meta intact. Twins
  that break a rule are left out; the rest sim paired at the run's iterations. The best mean over the best
  takes its place (it must beat the seed where the best does), with a warning naming the swap. Twins join the
  verified list, so a tie takes them to time to live.
- The Defense and hit floors come before the score. Gems are priced at the surrogate's slopes, and the gem
  check bans one gem at a time at those slopes, so where the slopes miss, neither fixes it: P1 picks gem 3
  Stamina in 11 to 13 sockets though a Stamina socket sims 4 to 8 times any other there.

## Resistance

A tank's gear resists nothing: it answers the one boss the tank fight models, not the rest of the phase.
The pool builder leaves out a tank's resistance-only enchants and gems (`isResistanceOnly`: fur linings,
resistance cloak enchants, Void Sphere), and the seed trimmer notes it. In Go a tank spec's pool (`tankSpec`)
prices items, socket bonuses, enchants and gems with resistance 0, and `dress` takes `gearResistance` back off
the bonus stats of every sim, sheet and the healing pin. Raid auras, talents and racials keep theirs; the boss
keeps its magic.

## Measuring

`TestMeasureTankModel` (worn gear: death chance, margin, time to live, normalizers at Normal and Quick, dJ per
stat point by term, margin per Stamina point against its health), `TestMeasureTankPicks` (the same at a batch
export's picks, plus the run's warnings, gems by kind and death chance against health),
`TestMeasureTankDeathAtPicks` (the death normalizer, coefficient and cap at the worn gear and at a pick) and
`TestMeasureTankDeathTerm` (the death normalizer under other Armor offsets, iteration counts and healing) sim
the requests the page posted, never one rebuilt from its saved raid: the save holds no rotation (`TypeAuto`
resolves as the page builds a request) and no raid buffs, so that tank only auto-attacks and dies far more often.

1. `python sim/optimizer/testdata/capture_requests.py tmp/requests --listen 3348 --server http://localhost:<sim port>`
   relays a browser to the sim server and saves each `/optimizeGearAsync` body; the page's run then fails at
   once. A tank saved without simple cooldowns sends defensives at 0 ([healing](#healing)): give it
   `rotation.simple.cooldowns.hpPercentForDefensives` 0.35 in a copy of the storage the driver restores
   (`<out>/storage-snapshot.json`), and check the captured requests.
2. Run the tanks through `http://127.0.0.1:3348`, by hand or with the
   [acbis driver](../../tools/database/acbis/driver/README.md)'s `--base`, on a Chrome `--port` and `--out`
   of its own.
3. `MSYS_NO_PATHCONV=1 tools/acore/dock.sh exec env BIS_REQUESTS=/wotlk/tmp/requests [BIS_PICKS=/wotlk/tmp/<batch-stage1.json>] go test -tags with_db -v -run 'TestMeasureTank(Model|Picks|DeathAtPicks|DeathTerm)' ./sim/optimizer/`

Requests and exports hold the roster: they go under the gitignored `tmp/`, which the container can read,
and are deleted afterwards, never committed.

## Hit floor

A tank run's only cap floor is melee hit (`capRoleTank` in `caps.go`): enough to bring `PhysicalMissChance` to
0, bisected on the sheet so talents and racials count, 262 rating bare. No expertise floor. It stays there by
the user's call: the tanks glyph their taunts. A tank run without its class's taunt glyph warns with the
miss chance at the floor (`warnTauntGlyph`).

That isn't a taunt cap. Taunt 355, Growl 6795, Dark Command 56222 and Hand of Reckoning 62124 are
`DmgClass MAGIC`: they roll spell hit (17% miss on a boss, 26.23 rating a point) and are never dodged or
parried. The floor's 262 rating is 10 points of it and the taunt glyphs' 8 cover the rest. Unglyphed, a taunt
caps at 446 rating, 367 under Misery or Improved Faerie Fire.
