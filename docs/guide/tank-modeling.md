# Tank modeling

What the BiS optimizer's tank runs fight and how they score (`ui/core/optimizer/pool_builder.ts`,
`sim/encounters/optimizer_tank_boss.go`, `sim/optimizer/{objective,caps,api}.go`). The tank suites' goldens use
the melee-only `GenericBossTarget` instead, with Patchwerk's numbers.

## The boss

`TANK_BOSSES` holds one boss per content phase; `TestTankBossesMirrorUI` keeps the Go mirror equal.

| Phase | Boss | Damage armor and avoidance can't remove |
|---|---|---|
| 1 | Kel'Thuzad 25 | Frostbolt 55802 (half land), Frostbolt volley 55807 |
| 2 | Algalon 25, dual wield | Phase Punch 64412, Black Hole Explosion 65108 |
| 3 | Anub'arak 25H | Freezing Slash 66012, Penetrating Cold 68510 |
| 4 | The Lich King 25H, `suppressDodge` | Soul Reaper 73799 and its tick, Infest 73781 |
| 5 | Halion 25H | Flame Breath 74528 |

- The target is `OptimizerTankBossID`, a preset with an AI and no preset encounter. It swings from the boss's own
  `creature_classlevelstats` row (`damageExp2`, `attackPower`) times `damageModifier`, with an off hand under
  `dualWield`.
- Each ability is 8 `target_inputs` numbers: spell id, school, damage, period min and max, cast seconds, land
  chance, start. The AI casts it with `OutcomeAlwaysHit` at a period rolled between the two, so only the
  school's resistance and damage-taken multipliers cut it.
- A cast with a cast time holds the boss's melee. A cast that fails its land chance is skipped and holds nothing:
  the raid interrupted it or it went to another player.
- Kel'Thuzad's `landChance` 0.5 on Frostbolt is the share the raid doesn't interrupt (the user's call). His
  first 228 s, where he doesn't attack, and his adds are left out.
- Server data only: an ability goes in when the boss script and Spell.dbc give it a fixed number on the current
  tank. The server strips Physical from a mixed-school spell, so Freezing Slash and Soul Reaper are pure magic.
- Not modelled: Halion's Twilight Precision (tank dodge × 0.8, +5% hit; core's `EnemyDodgeMultiplier` has no
  setter), abilities armor still cuts (Algalon's Quantum Strike), ones that follow the tank's health (Frost
  Blast) or position.

## Healing

`healingModelForBoss` leaves `hps` 0, so `pinHealingModel` sets it once per run to 1.5 × the DTPS of the gear
as equipped (not the trimmed seed, which a small pool leaves nearly naked). Cadence is 1.5 boss swings, varied by
`TANK_HEAL_CADENCE_VARIATION` (0.5) of itself, so the tank can die.

## Score

- `tankMetricWeights` and `TankWeights` give threat its slider share and split survival in thirds: DTPS, TMI,
  chance of death. The Advanced editor overrides them.
- Armor is the reference stat for the three; its `normalizerOffset` is 2000 so chance of death clears 2 se at
  Normal. A metric that doesn't is dropped with a warning.
- Chance of death has no per-iteration samples, so its delta between two loadouts isn't paired. Near the optimum
  picks die in under 1% of iterations and only TMI prices Stamina.

## Taunt hit floor

A tank run's only cap floor is melee hit (`capRoleTank` in `caps.go`): enough to bring `PhysicalMissChance` to
0, bisected on the sheet so talents and racials count, 262 rating bare. Taunt, Growl, Dark Command and Hand of
Reckoning roll the melee special-attack miss (8% against level 83) on the server; spell hit doesn't help. No
expertise floor: a taunt's dodge and parry would need about 671 rating.
