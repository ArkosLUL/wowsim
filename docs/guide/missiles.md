# Missiles

How long a missile flies and when it lands. Declaring a spell's missile speed:
[spell data and timing](spell-data-and-timing.md#server-data); the update grid a landing rides:
[update timing](spell-data-and-timing.md#update-timing).

## Flight and reach

- Missiles roll hit, crit and damage at launch (`Spell::DoAllEffectOnLaunchTarget`) and deal it on landing, in
  whole ms. The flight is the distance less the caster's reach, at least 5 yd (`Spell::AddUnitTarget`;
  `Spell.TravelTime`): `GetDistance` takes off the caster's object size only, never the target's.
- The reach is 1.5 for a player, a pet's or guardian's [its own](pets.md#reach-and-position), times the scale
  (`Unit.combatReach()`, `PseudoStats.ScaleBonus`): Bloodlust and Heroism carry `SPELL_AURA_MOD_SCALE` 30%, so
  under them a player's is 1.95, and a speed 24 missile at 18 yd flies 668 ms, not 687. A script writing the
  reach holds until the scale next changes (`Unit.SetCombatReach`).
- The sim waits it out only where the closure calls `spell.WaitTravelTime`, or `spell.DealDamageAfterTravel` for
  a landing that only deals the result, over `DistanceFromTarget`: 0 for a pet that doesn't cast from a spot of
  its own, so its missiles wait the 5 yd floor.
- To a friendly unit (`WaitTravelTimeTo`, `TravelTimeTo`) the distance is the gap between both units' distances
  from the target, the sim placing units only by that; to the caster himself the flight is 0, though it still
  lands on an update as below.
- A target's combat reach is `Target.combat_reach` (`proto/common.proto`; "Combat Reach" in the advanced
  target picker), unset the boss dummy's 2.5 (model 27510, `core.BossDummyCombatReach`), read as
  `Unit.EnemyCombatReach`. Ranges add both reaches (`Spell::GetMinMaxRange`); only Mirror Image's ranges
  ([pets](pets.md#guardian-ais)) and a hunter pet's spot ([pets](pets.md#reach-and-position)) read it.

## Landing

- A missile lands on its caster's first update at or after its flight's start plus the flight (`Spell.landsAt`;
  the server re-plans its `SpellEvent`): a world tick for a player, the map update for a creature, pet or
  guardian. The flight starts on the caster's first update at or after the launch, or the one after when
  launched in an update or caster event pass, any unit's for a player (creatures update after every player),
  only its own for anyone else: `sim.updating` (Auto Shot, a dot tick, a creature AI's instant) or
  `sim.eventPass` (a landing, a `DelayedPeriodicApplier` action, a hardcast's completion bar its own missile).
  - A copy cast off another unit's landing through a spell without a cast config (the Rune Weapon's Death
    Coil) starts on the update at or after it. A creature's cast through a cast config sets `sim.updating` to
    it, so that missile always starts an update later.
- A totem's spell (`CastFromAnotherUnit`; Searing Totem's 58702 is the totem's 2.2 s hardcast) starts on the map
  update at or after the launch and lands on the map update, outside the shaman's own landings. Any missile on
  exact timing lands at launch plus the flight.
- A hardcast due on a landing's update completes first (`landingAction.land` runs `landDueCast`): `m_Events`
  keys it at the update before + 1 ms, the landing at go + flight. The caster's other landings due on that
  update then run by key ahead of it (`dueLandingAhead`), a tie in launch order. Other units' actions at that
  time keep queue order.
- `sim.QueueSpellEvent` runs a callback as one of the caster's `m_Events` on its update, keyed 1 ms past the
  update before: a cast with no cast time waiting on its `SpellEvent` (Penance's channel), a script's 1 ms
  event (Combustion's stack). A hardcast due then still completes first.
