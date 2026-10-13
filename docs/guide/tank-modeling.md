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
- The AI casts an ability with `OutcomeAlwaysHit` at a period rolled between its two, so only the school's
  resistance and damage-taken multipliers cut it, and armor a `Physical` one. That is a `DmgClass NONE` spell
  on the server (`MagicSpellHitResult`, no block); `SPELL_ATTR3_NO_AVOIDANCE` is never read.
- `followUpSeconds` and `followUpDamage` are a periodic aura's one tick after the cast, a DoT of it.
  `followUpHaste` multiplies the boss's melee speed for `followUpHasteSeconds` as it ticks (Soul Reaper's 69410).
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

## Score

- `tankMetricWeights` and `TankWeights` give threat its slider share and split survival in thirds: DTPS, TMI,
  chance of death. The Advanced editor overrides them.
- Armor is the reference stat for the three, with a `normalizerOffset` of 2000. A metric whose normalizer
  doesn't clear 2 se is dropped with a warning.
- Chance of death pairs like the others: core lists each iteration's death (`UnitMetrics.iteration_deaths`,
  under `SaveAllValues`, for a unit that tanks). Where a pick all but never dies (Anub'arak), the death third
  is flat and only TMI prices Stamina.
- Where the gear as equipped all but always or never dies, the death normalizer doesn't clear 2 se and the
  run drops the term with a warning. The raid's gear all but never dies on Kel'Thuzad and always on the Lich
  King, and no gear in either pool changes that: half the Lich King's iterations survive only at 81,000 to
  96,000 health. On the Lich King neither ±8000 Armor, 16,000 iterations nor healing at 0.75 to 4 times DTPS
  moves it off 100%; on Kel'Thuzad ±8000 Armor or healing at 1.0 times DTPS brings the term back, and 1.0
  kills the same gear on Algalon in 87 to 99%.
- A normalizer that clears 2 se by little (Halion, worn gear dying in 99.7 to 99.9%) keeps the term with a
  noisy, oversized coefficient: death chance swamps the score, and the stat curves measured at that gear can
  be noise.
- The Defense and hit floors come before the score: a P1 pool has almost no Defense, so its gems go there
  though a Stamina socket is worth 1.4 to 2.6 times any other.

## Measuring

`TestMeasureTankModel` (worn gear: death chance, normalizers, dJ per stat point), `TestMeasureTankPicks`
(the same at a batch export's picks, plus death chance against health) and `TestMeasureTankDeathTerm` (the
death normalizer under other Armor offsets, iteration counts and healing) sim the requests the page posted,
never one rebuilt from its saved raid: the save holds no rotation (`TypeAuto` resolves as the page builds a
request) and no raid buffs, so that tank only auto-attacks and dies far more often.

1. `python sim/optimizer/testdata/capture_requests.py tmp/requests --listen 3348 --server http://localhost:<sim port>`
   relays a browser to the sim server and saves each `/optimizeGearAsync` body; the page's run then fails at
   once.
2. Run the tanks through `http://127.0.0.1:3348`, by hand or with the
   [acbis driver](../../tools/database/acbis/driver/README.md)'s `--base`, on a Chrome `--port` and `--out`
   of its own.
3. `MSYS_NO_PATHCONV=1 tools/acore/dock.sh exec env BIS_REQUESTS=/wotlk/tmp/requests [BIS_PICKS=/wotlk/tmp/<batch-stage1.json>] go test -tags with_db -v -run 'TestMeasureTank(Model|Picks|DeathTerm)' ./sim/optimizer/`

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
