# Spell data and timing

How the sim applies the server's spell data and update timing. The rest of where the server differs is in
[server parity](server-parity.md).

`RegisterSpell` applies the server's own data for the spell it's given (`Spell.ServerSpell()`), so a
spell is only as right as its id.

- **Flags** take the server's value for Binary, NoActiveDefense, CompletelyBlocked and Channeled, and
  only gain AlwaysHit, the ATTR7 no-dodge and no-parry bits, and IgnoreResists (ATTR4_NO_CAST_LOG, on
  non-physical spells). The yellow roll skips its whole table for AlwaysHit, crit aside.
- **Timing** applies to what the spell declares: cast time against `CastMs` (which already carries the
  ranged slot's +500 ms), GCD against `GCDMs` in category 133, and the cooldowns. A declared value
  inside what passive talents and glyphs can reach agrees; anything else is a conflict, and an
  allowlist entry either lets the server win or keeps the sim's value with a reason.
- **Damage** is declared per server effect: `core.SpellEffect` on `SpellConfig.Direct` and `DotConfig.Tick`
  (`sim/core/spell_effect.go`), with talent and glyph mods in `SpellConfig.Mods`, folded as
  `Player::ApplySpellMod` does. `RegisterSpell` checks the level-80 base range, the coefficients and the
  weapon percent against the server's; school and missile speed sync like timing. A class spell with server
  damage but no declaration goes on its class's `serverdata_undeclared.go`, and `TestServerDataConflicts`
  fails on a missing or stale entry. Declaring:
  - A pet's or guardian's spell takes its owner's spell mods (auras 107/108) where the class mask matches
    (`Unit::GetSpellModOwner`), charged ones too except on a guardian's cast time. The owner's other auras
    don't reach it, e.g. school crit (71) or a meta gem's crit damage (163).
  - A physical strike's fixed bonus takes its hand's TOTAL_PCT, 0.5 in the off hand; other schools don't
    (`Spell::EffectWeaponDmg`). The sim's helpers: `sim/{deathknight,rogue,warrior}/weapon_strikes.go`.
  - Damage a script hands over (`CastCustomSpell`) is dealt exactly, with no roll.
  - Below its MaxLevel, a spell's spell power part scales by (SpellLevel + 6) / level, at most 1
    (`Unit::CalculateLevelPenalty`).
  - A talent's effect mod is flat (aura 107) or a percent (108): read Spell.dbc, not the tooltip.
  - The undeclared list misses damage a script deals behind a dummy effect (Execute, Death and Decay, Holy
    Shock) and proc-damage and damage-shield auras (Holy Shield): declare those anyway.
  - Aura 227 (`SPELL_AURA_PERIODIC_TRIGGER_SPELL_WITH_VALUE`) hands its amount to the spell it triggers as
    base points: Mind Flay's channel gives each tick its 196.
- **Wrapper ids** are the trap. The sim often deals damage under the id of a spell that, on the server,
  only triggers the real one (totems, Faerie Fire (Feral), Typhoon). The wrapper's
  flags are not the damage's, so those entries keep the sim's values until the class's P7 item moves
  the damage to the real id. When a suite moves for a spell you didn't touch, look here first.
- Missiles roll hit, crit and damage at launch (`Spell::DoAllEffectOnLaunchTarget`) and deal it on landing,
  after at least 5 yards of travel (`Spell::AddUnitTarget`), in whole ms. The sim waits out the travel only
  where the closure calls `spell.WaitTravelTime`, or `spell.DealDamageAfterTravel` for a landing that only
  deals the result, over `DistanceFromTarget`, which is 0 for a pet: its missiles wait the 5 yd floor.

The server only acts on its map update, 100 ms live (`ServerSettings.MapUpdateInterval`; 0 means exact,
which is what unit tests want).

- Swings, hardcast completions and aura expiry land on the next tick, with a phase rolled per iteration.
  A swing restarts its timer from the tick it landed on, so the overshoot is lost, which costs a fast
  dual wielder several percent.
- A landed swing pushes the other hand to at least 200 ms. A melee swing restarts the ranged timer, and
  every Auto Shot restarts both melee timers (`Unit::_UpdateAutoRepeatSpell`).
- At the pull, and when auto attacks restart (breaking stealth), a ready off hand starts half the main hand's
  hasted attack time behind it (`Unit::Attack`).
  A weapon swap doesn't reset swings.
- A cast resets every swing timer when the server's `ResetsAutoAttack` says so. A triggered cast never
  resets, nor does one made instant or one an `SPELL_AURA_IGNORE_MELEE_RESET` aura covers (Maelstrom
  Weapon).
- A swing due during a hardcast waits for the update the cast lands on and goes after its effects; a cast
  started off that landing comes a later update, so it can't hold the same swing again. Melee
  waits out any hardcast, Auto Shot only one without `SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS`. With that
  attribute a player's melee timers stand still through the cast instead (Slam). A cast started by the
  main hand's last APL check comes before that swing: the server handles the session before
  `Player::Update`. A `CancelAutoSwing` from that check stops the swing. A haste change rescales only the
  time left on a frozen timer.
- A channel holds melee too. In the sim a channel opts in with `Spell.HoldMeleeUntil` (Army of the Dead;
  Bladestorm isn't a server channel), and Volley stands the ranged timer still (`suspendRangedAttackTimer`).
- The GCD follows `Spell::TriggerGlobalCooldown`: hasted only with `HasteGCD`, then kept within
  [1000, 1500] ms. Cast time scales by damage class: spell haste for magic, ranged attack speed for
  ranged, nothing for melee. Hasted GCDs and cast times truncate to whole ms before the clamp and the
  tick (`Unit::ModSpellCastTime`), which can save a tick.
