# Tank modeling

What the BiS optimizer's tank runs fight and how they score (`ui/core/optimizer/pool_builder.ts`,
`sim/encounters/optimizer_tank_boss.go`, `sim/optimizer/{objective,caps,api}.go`). The tank suites' goldens use
the melee-only `GenericBossTarget` instead, with Patchwerk's numbers.

## The boss

`TANK_BOSSES` holds one boss per content phase; `TestTankBossesMirrorUI` keeps the Go mirror equal.

| Phase | Boss | Damage armor and avoidance can't remove |
|---|---|---|
| 1 | Kel'Thuzad 25 | Frostbolt 55802 (half land), Frostbolt volley 55807 |
| 2 | Algalon 25, `dualWield`, `noParryHaste` | Phase Punch 64412, Black Hole Explosion 65108 |
| 3 | Anub'arak 25H, `noParryHaste` | Freezing Slash 66012, Penetrating Cold 68510 |
| 4 | The Lich King 25H, `noParryHaste`, `suppressDodge` | Soul Reaper 73799 and its tick, Infest 73781 |
| 5 | Halion 25H | Flame Breath 74528 |

- The target is `OptimizerTankBossID`, a preset with an AI and no preset encounter. It swings from the boss's own
  `creature_classlevelstats` row (`damageExp2`, `attackPower`) times `damageModifier`.
- `dualWield` adds an off hand without the dual wield miss penalty: Algalon's Dual Wield 42459 gives 20% hit
  beside it.
- `noParryHaste` is the row's `CREATURE_FLAG_EXTRA_NO_PARRY_HASTEN`.
- Each ability is 9 `target_inputs` numbers: spell id, school, damage, period min and max, cast seconds, land
  chance, start, resets swing. The AI casts it with `OutcomeAlwaysHit` at a period rolled between the two, so
  only the school's resistance and damage-taken multipliers cut it. A page loaded before a change to the group
  sends the old one and the AI panics: reload it.
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
- Not modelled: Halion's Twilight Precision (tank dodge × 0.8, +5% hit; core's `EnemyDodgeMultiplier` has no
  setter), Soul Reaper's 69410 (the Lich King's melee haste after the tick), abilities armor still cuts
  (Algalon's Quantum Strike), ones that follow the tank's health (Frost Blast) or position.

## Healing

`healingModelForBoss` leaves `hps` 0, so `pinHealingModel` sets it once per run to 1.5 × the DTPS of the gear
as equipped (not the trimmed seed, which a small pool leaves nearly naked). Cadence is 1.5 boss swings, half
that under `dualWield`, varied by `TANK_HEAL_CADENCE_VARIATION` (0.5) of itself, so the tank can die.

## Score

- `tankMetricWeights` and `TankWeights` give threat its slider share and split survival in thirds: DTPS, TMI,
  chance of death. The Advanced editor overrides them.
- Armor is the reference stat for the three; its `normalizerOffset` is 2000 so chance of death clears 2 se at
  Normal. A metric that doesn't is dropped with a warning.
- Chance of death has no per-iteration samples, so its delta between two loadouts isn't paired. Near the optimum
  picks die in under 1% of iterations and only TMI prices Stamina.

## Hit floor

A tank run's only cap floor is melee hit (`capRoleTank` in `caps.go`): enough to bring `PhysicalMissChance` to
0, bisected on the sheet so talents and racials count, 262 rating bare. No expertise floor.

That isn't a taunt cap. Taunt 355, Growl 6795, Dark Command 56222 and Hand of Reckoning 62124 are
`DmgClass MAGIC`: they roll spell hit (17% miss on a boss, 26.23 rating a point) and are never dodged or
parried. The floor's 262 rating is 10 points of it and the taunt glyphs' 8 cover the rest. Unglyphed, a taunt
caps at 446 rating, 367 under Misery or Improved Faerie Fire.
