# Spell data and timing

How the sim applies the server's spell data and update timing. A missile's flight and landing are in
[missiles](missiles.md), the rest of where the server differs in [server parity](server-parity.md).

## Server data

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
  - A physical strike's fixed bonus takes its hand's TOTAL_PCT, 0.5 in the off hand, where a death knight's,
    flat mods included, is doubled first: the DBC already halves Threat of Thassarian's (`offHandStrikeFixed`).
    Other schools take neither (`Spell::EffectWeaponDmg`). The sim's helpers: `sim/core/weapon_strike.go` and
    `sim/{deathknight,rogue,warrior}/weapon_strikes.go`.
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

## Update timing

The server updates players every world tick, and creatures, pets and guardians, with the auras they carry, only
on the map update, 100 ms live (`Map::Update`): the first world tick its interval is up by, at least 3 ticks
apart (`MapMgr::Update`). `MinWorldUpdateTime` (10 ms live) is only a tick's floor: the loop sleeps the rest of
it and wakes late, by a different amount each time, and one tick of every update runs long, so an update's
length scatters around a mean above its interval.

The sim draws every map update's length, per iteration (`sim/core/update_grid.go`). Four `ServerSettings`
shape it:
- `world_loop_ms`, 10.44 live: a world tick's mean length, so a map update's ten average 104.4 ms.
- `map_update_jitter_ms`, 5 live: the standard deviation of one update's length, normal around that mean.
- `world_long_tick_ms`, 39.5 live, and `world_long_tick_jitter_ms`, 6.5 live: an update is short ticks of
  `world_update_interval_ms`, one long tick and one short tick after it, the next update. Each update's long
  tick aims at its own target, normal around the setting with that standard deviation and unrelated to the
  update's length (`longTickPairing`), and takes what the drawn length leaves after as many short ticks as bring
  it nearest the target: one tick in about seven, the longest 59 ms. Taking whole short ticks off spreads it
  about 2.9 ms alone, so a jitter up to that adds nothing. `MapMgr::Update` runs the open world, battlegrounds
  and instances on three ticks in a row, the first runs long under the bots, and an instance updates one tick
  after it: the cycle is 100 ms plus what the long tick leaves over a multiple of 10, which is the 104.4, and an
  instance's update can come under 100 ms after the last. A player's event waits 10.8 ms for the next tick on
  average, 5.2 ms on even ticks (live swings land 10.8 ms past their attack time).

None is a config key. They're what the host does under its load, fitted to recordings as constants in
`tools/acore/gen_server_defaults/settings.go` ("World loop time", "Map update jitter", "World long tick" and
"World long tick jitter" in the UI): a test run on the host adds about a ms to the update and the long tick
grows with the worldserver's load, so record a period-sensitive number on an idle host. `TestEcCore6WorldTick` measures the ticks
([server validation](server-validation.md#live-e2e)).

The long tick at 0 gives a map update ten even world ticks, drawn above a 100 ms floor (lognormal, whatever
`map_update_jitter_ms` says). `map_update_jitter_ms` or `world_loop_ms` at 0 gives a fixed grid with a phase
rolled per iteration, `world_loop_ms` 0 one of `world_update_interval_ms` ticks. A map interval of 0 makes every unit
exact, which is what unit tests want. Outside the fight every update takes the mean, in even ticks.

- Ask the grid, never step by an interval: `sim.NextServerTick` and `NextWorldTick` give the first update at or
  after a time, `ServerTickBefore` and `WorldTickBefore` the last one before it. With jitter
  `serverTickInterval` holds only the shortest update. A lookup reads a bucket table (`updateJitter.tab`): a
  bucket is a power of two no wider than the shortest update, so a time is in its bucket's update or the one
  after (`TestUpdateTableAtTheEdges`). Each update draws one step off the seed, in order
  (`TestEvenTickUpdatesAreDrawnLikeTheCursorWalk`).
- Riding on the grid: a creature's 2 s timer runs out on its 19th update or its 20th, about 2064 ms on average, and
  its 3 s cast takes 3.00 to 3.14 s. An Ignite application due 2 s or 4 s after the last races the old
  Ignite's tick on one update and wins a little over half the time; a fixed grid decides each race one way,
  so the fire mages' goldens move with both settings.
- Swings, Auto Shot and hardcast landings land on the caster's next tick. A swing restarts its timer from the
  tick it landed on, losing the overshoot: up to a world tick for a player, a map update for a pet.
- Auras and dots tick and expire on the grid of what counts them down: the unit they sit on, a channel's
  caster, or the map update for an area dot from a dynamic object, totem or trap (effects 27, 28, 105).
- An aura expires at its own time (`Simulation.Step`), ahead of anything queued for then but what
  `Unit::Update` runs before a player's aura pass: its cast landings and missile hits (`m_Events`;
  `ActionPriorityEvent`), then its Auto Shot. Its melee follows the pass (`Player::Update`), a swing a cast held
  included (`WeaponAttack.releasing`).
- On one update every player goes before the creatures (`Map::Update`): the players' auras, a player's channel
  and self hot ticks too, then their melee, then the creatures' auras (`Step`; the players' attacks lead
  `sim.weaponAttacks`). A unit's due ticks go, lowest spell id first, before its expiries
  (`Unit::_UpdateSpells`; `Unit.tickingDots`): Ignite (12654) ticks before Pyroblast's DoT (42891).
- A creature's own update runs its due hardcast landing, then the auras on it, then its AI's instant cast and
  its swing (`Creature::Update`): the landing still finds an aura running out on that update
  (`Unit.landDueCast`), and the cast or swing comes behind the ticks due with it (`target_ai.go`,
  `WeaponAttack.swing`). Its landings keep their queue order against those auras.
- A periodic action with `PeriodicActionOptions.Owner` is one of that unit's periodic ticks: on its first
  update after each period, the remainder carried, ahead of a player's expiries, and an aura it puts on counts
  from that update (Prey on the Weak's check).
- An execute phase by time starts at its own time. One by damage starts on the target's first map update from
  the crossing damage on, where the server sets the under 35% and under 20% aura states (`Unit::Update`;
  `executeDamageReached`). 25% starts with its damage: only Drain Soul reads it, off the health.
- An aura put on outside its unit's own update counts down from the update before (`Aura.countdownStart`):
  - A creature's from the map update before (`Map::Update` hands `_UpdateSpells` the whole update's diff), so
    the dummy's dots tick and expire up to one update early.
  - A player's from the world tick before: a cast or landing comes ahead of the player's update in its tick.
    One put on from a creature's or pet's update counts from that world tick.
  - One the unit puts on itself in its own update (swing, AI instant, dot tick, expiry) counts from that
    update, and so does an area dot's object made in a map update.
  - So time left never equals the duration on a fresh aura: ask `Aura.RefreshedNow`. An aura that must still
    be on for an action a whole duration after its cast (a periodic action's last tick) takes
    `Unit.DurationThroughLastTick`.
- A dot stops at max duration / amplitude ticks (`AuraEffect::Update`, `GetTotalTicks`; `Dot.TotalTicks`), its
  next tick owed until a refresh: `Dot.Rollover`, `Refresh` and `Activate` restart the count.
  - With `SPELL_ATTR5_EXTRA_INITIAL_PERIOD` a new aura ticks as it goes on, one tick more in all; a refresh
    doesn't. `core.Dot` doesn't read the attribute: each such spell ticks on apply itself (`Dot.TickOnce`, or
    `TickImmediately` for Killing Spree's periodic action): Bladestorm, Death and Decay, Explosive Shot,
    Penance, Poison Spit.
  - A dot arms its last tick action again once nothing queued can run it (`Dot.spareTick`), even under its own
    running tick, so a tick tells it was re-armed by `tickArmings`, never by `tickAction == pa`.
  - Lengthen a running aura with `Aura.AddDuration`, a script's `SetDuration(GetDuration() + extra)`: the time
    goes on what the duration has left (`Aura.expiresNominal`), not on the update it would have ended on, so
    it ends on the first update that far on. `UpdateExpires` ends it at the time given, off the grid unless
    that's an update; `Dot.UpdateExpires` moves the max duration with it.
  - `Dot.AddDuration` raises the max duration too, so the added time ticks (Rip under Shred's glyph, Moonfire
    under Starfire's); `Aura.AddDuration` alone leaves the tick count (stock Glyph of Backstab).
  - `Dot.AddRemaining` adds to the time left and raises the max duration only when the new time passes it
    (`spell_sha_t10_elemental_4p_bonus`). A dot whose duration shrinks with haste (`SpellHasteScalesBoth`)
    keeps its max duration in unhasted intervals, so the hasted time left is converted first.
- A munched periodic (Deep Wounds, Unholy Blight, Piercing Shots, Righteous Vengeance, Ignite, Languish) applies
  on the next 400 ms step of the caster's event clock, up to 400 ms late (`CalculateQueueTime`,
  `MunchingBlizzlike.Enabled`), ahead of a swing or tick due on the same server tick
  (`core.DelayedPeriodicApplier`, a phase per caster per iteration).
  - Fed by a hit the session deals with the player's own instant cast (Fire Blast), it draws the step off a
    clock one world tick stale, since the session pass runs before `Player::Update`, so it can land that
    update (`ApplyFromInstantCast`).
  - Any other hit reads the current clock, as a hardcast's landing does: a missile's (a Hot Streak
    Pyroblast) or one off a tick, a swing or an aura's end (Living Bomb's explosion). Ignite tells them apart
    with `SpellResult.DealtWithItsCast`.
- An extra attack (Hand of Justice, Sword Specialization) is a plain swing: it never takes a queued melee spell
  such as Heroic Strike or Rune Strike (`Unit::AttackerStateUpdate`'s `extra`).
- A landed swing pushes the other hand to at least 200 ms. A melee swing restarts the ranged timer, and
  every Auto Shot restarts both melee timers (`Unit::_UpdateAutoRepeatSpell`).
- At the pull, and when auto attacks restart (breaking stealth), a ready off hand starts half the main hand's
  hasted attack time behind it (`Unit::Attack`).
  A weapon swap doesn't reset swings.
- A cast resets every swing timer when the server's `ResetsAutoAttack` says so. A triggered cast never
  resets, nor does one made instant or one an `SPELL_AURA_IGNORE_MELEE_RESET` aura covers (Maelstrom
  Weapon). A swing after its unit's hardcast lands comes an update short of the attack time, a world tick for
  a player: the cast lands in `Unit::Update`'s event pass, ahead of that update's attack timers (`cast.go`). An
  instant the session casts loses the same tick; one cast from the unit's own update keeps it.
- A swing due during a hardcast waits for the update the cast lands on and goes after its effects; a cast
  started off that landing comes a later update, so it can't hold the same swing again. Melee
  waits out any hardcast, Auto Shot only one without `SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS`. With that
  attribute a player's melee timers stand still through the cast instead (Slam). A cast started by the
  main hand's last APL check comes before that swing: the server handles the session before
  `Player::Update`. A `CancelAutoSwing` from that check stops the swing. A haste change rescales only the
  time left on a frozen timer.
- A channel holds melee too. In the sim a channel opts in with `Spell.HoldMeleeUntil` (Army of the Dead;
  Bladestorm isn't a server channel), and Volley stands the ranged timer still (`suspendRangedAttackTimer`).
- A hasted channel's duration and tick period truncate each on its own (`int32(x × cast speed)`), so its aura
  can end on the caster's update after the last tick. The rotation comes back from the aura's end, never from
  a tick: with the GCD over, the next action is queued `ChannelClipDelay` later (`newDot`).
- The GCD follows `Spell::TriggerGlobalCooldown`: hasted only with `HasteGCD`, then kept within
  [1000, 1500] ms. Cast time scales by damage class: spell haste for magic, ranged attack speed for
  ranged, nothing for melee. Hasted GCDs and cast times truncate to whole ms before the clamp and the
  tick (`Unit::ModSpellCastTime`), which can save a tick.
