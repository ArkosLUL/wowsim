# Caster mechanics

Spells, talents and procs of the druid, shaman, mage, priest and warlock where the server differs from what
the Classic sim did, and the sim now follows it. The death knight, warrior, paladin, rogue and hunter are in
[class mechanics](class-mechanics.md). Core rules (costs, threat, procs) are in
[server parity](server-parity.md), damage and heal amounts in [damage and healing](damage-and-healing.md),
pets and guardians in [pets](pets.md).

## Druid

- **Eclipse** procs only on crits (spell_proc -48516 HitMask 2): `spell_dru_eclipse` rolls the ProcChance (×0.6
  for Wrath), then spell_proc's chance; never while either eclipse is up.
- **Vengeance** covers Wrath, Moonfire, Starfire and Starfall, not Insect Swarm.
- **Rake**'s first hit takes armor.
- A missed cat or bear strike pays the [miss cost](class-mechanics.md#miss-cost), Primal Precision's cut
  included.
- **Forms:** cat and bear weapon damage is min(level, 60) × 0.85 / 1.25 × the unhasted attack time; a weapon's feral
  AP truncates to int32 in float32; Predatory Strikes gives 7/14/20% of that feral AP plus the item's own AP.
- **Starfall** (53201) is an aura on the druid and can't miss: 10 ticks, each casting a star (53195) at two
  different random units in sight, or at the one there is (`spell_dru_starfall_dummy`). A star casts its splash
  (53190) at launch, on the units around its target. Stars and splash roll Omen of Clarity, as do Hurricane's
  cast and ticks (measured live, 8.75%). The stars need line of sight to the target's centre.
- **Omen of Clarity** rolls 3.5 PPM over the spell's cast time entry, at least 1.5 s: 8.75% an instant, 11.67%
  Wrath (2 s). Any landed magic-class spell outside the druid's family that deals direct damage or heals and
  gives no power rolls it too, an item's proc included (`rollsOmenOffFamily`): Extract of Necromantic Power's
  bolt at 12.83%. Barkskin, Innervate and Rebirth roll (`CheckProc` passes them); Nature's Grace's 16886 and
  Survival Instincts roll nothing.
  - A magic-class heal rolls like a hit (`OnHealDealt`), an item's too: Bauble of True Blood's 71607, Althor's
    Abacus. A heal over time's ticks roll nothing, nor does a heal without a damage class (Glowing Twilight
    Scale's 75493).
  - A landed hit without damage rolls: Insect Swarm, Faerie Fire.
- **Nature's Grace** rolls its chance in the cast phase and again in the hit phase (spell_proc -16880): a crit
  Wrath gives 16886 at its launch and at its hit, a cast spell without a missile rolls both at its hit, a
  triggered one (Starfall's stars, Typhoon's 53227) the hit phase only (`rollNaturesGrace`). The hit phase rolls
  once for each target the cast crits. Its mask has
  Healing Touch, Regrowth, Nourish and Swiftmend too: the aura reads heals, so one needs only
  `SpellFlagNaturesGrace` (the sim registers none of the four).
- **Savage Defense** holds one charge, which the druid's crits refill at their own rate: the slower the boss
  swings, the fewer swings find it down.
- **Hurricane**'s cast is a hit without damage on the druid's side: one Illustration of the Dragon Soul stack,
  none from its ticks.
  - Its second effect slows the attacks of every creature in the storm by 20%, a boss's too, sharing the
    attack speed debuffs' stacking rule (spell_group 1059; `AtkSpdReduction`). On rank 5 (48467) that effect
    has no mechanic; ranks 1 to 4 carry `MECHANIC_SLOW_ATTACK`.
  - A snare-immune target holds the aura only half the time: `Aura::UpdateTargetMap` compares the effects a
    target has on with the storm's own, immune ones included, takes a mismatch off and puts it back on its
    next pass. A pass runs on the first map update where the updates since the last add up to 500 ms
    (`nextStormPass`), and the cast's own starts the aura on.
- **Faerie Fire (Feral):** a landed 16857 rolls Omen's 3.5 PPM over the 1.5 s an instant counts as (8.75%), and
  so does Bear Form's triggered 60089 (`CAN_PROC_FROM_PROCS`). With `OmenClarityFaerieFire` the script casts
  Clearcasting (16870) itself, and that roll still runs: only it gives the Balance T10 2-piece proc (70721).
- **Force of Nature** casts 33831, inside Clearcasting's mask.
- **Nature's Swiftness** makes instant what its aura's class mask covers (`Spell.AffectedByAuraEffect`): heals and
  Rebirth, not Starfire.
- **Survival Instincts** is [bonus health](class-mechanics.md#bonus-health).
- The Feral Tank's automatic rotation is its default APL, whose `autocastOtherCooldowns` casts Survival
  Instincts, Frenzied Regeneration and Barkskin.

## Shaman

- **Lava Lash** takes the whole off hand and, a melee-class spell, crits ×2, outside Elemental Fury's mask.
- **Enhancing Totems:** Flametongue Totem's 58655 is a raid area aura, so its int32(144 × 1.15) = 165 reaches every
  raid member and pet.
- **Improved Healing Wave** takes 0.1 s a point off Healing Wave.
- **Frost Witch's 4-piece** (`spell_sha_t10_elemental_4p_bonus`): a Lava Burst cast adds 6 s to Flame Shock's
  time left. The max duration, which caps the ticks, rises only when the new time passes it
  ([`Dot.AddRemaining`](spell-data-and-timing.md#update-timing)).

## Mage

- **Torment the Weak** counts only against a snared or attack-slowed target, whoever did it: raid bosses and
  the dummy are immune to snares ([mechanic immunity](server-parity.md#other-measured-server-behaviour)), not
  `MECHANIC_SLOW_ATTACK`: there only an attack speed debuff with that mechanic counts (`attackSlowed`: Thunder
  Clap, Frost Fever, Infected Wounds, Judgements of the Just). A landed Frostbolt (not under Glyph of
  Frostbolt) or Frostfire Bolt (9 s), or Chilled (1.5 s), slows any other target, Permafrost included; so do a
  Mirror Image's Frostbolt, Blast Wave (6 s) and Dragon's Breath (5 s, gone at the first direct damaging hit:
  one proc charge). Frostbite is a root.
  - Another unit's snare counts through an aura tagged `core.SnareAuraTag`: another mage's chill, Frost Shock
    (8 s), Mind Flay, a landed Typhoon (53227, 6 s), Hamstring, Slow.
  - Rank 5 Hurricane, the sim's, counts only where its snare effect lands, so never on a snare-immune
    target: its attack slow has no mechanic.
- **Slow** (31589) lasts 15 s on one target at a time: cast time ×1.3 and ranged attack time ×1.6 on any target,
  the strongest cast time debuff counting (spell_group 1096; `CastTimeDebuff`), and the snare only where
  snares land. It's outside Clearcasting's and Arcane Power's masks, so it always pays in full, and Arcane
  Concentration takes damage only: no `SpellFlagMage`.
- **Brain Freeze** rolls on landed Frostbolts, Frostfire Bolts and Blizzard ticks (spell_proc -44546). Fireball!
  (57761) lasts 15 s and is used at the cast.
- **T10's 2-piece** triggers when Hot Streak or Missile Barrage is used up, never on a timeout. Missile Barrage
  goes at Arcane Missiles' finish phase.
- **Fingers of Frost** rolls at the cast (spell_proc 44545), hit or miss; Improved Blizzard rolls its own on each
  landed tick. Only spells in 74396's mask spend charges: one a cast, none a channel's cast, one a landed hit of
  a triggered spell (Blizzard's ticks, Arcane Missiles' missiles, Living Bomb's explosion). Shatter's crit goes
  to spells in both Shatter's and 44544's masks: not item procs, not Blizzard.
- **Precision** counts once on Frostfire Bolt; **Arcane Focus**'s hit covers the missiles (42845), not the
  channel cast (42846).
- **Enduring Winter:** a landed Frostbolt, at most every 6 s (spell_proc -44557). **T8's 4-piece** is 20%.
- **Master of Elements** refunds 10% a point of the truncated base cost on any fire or frost crit, whatever the cast
  paid; **Burnout** burns apart. Each at most once per 8 ms (spell_proc -29074, -44449), so a crit on several targets
  refunds once.
- **Blizzard**'s tick (42938) is a proc: only `CAN_PROC_FROM_PROCS` auras take it.
- **Pyroblast's DoT** with Empowered Fire:
  - Its ticks crit whatever the switches (aura 286 on the talent is `spell_dbc` data), at the cast's crit
    chance, snapshotted at the landing. A crit tick feeds Ignite; Hot Streak, Master of Elements, Combustion
    and Burnout take no tick.
  - With `PyroblastEmpoweredFire` the tick is min(3 s × cast speed, 3 s) in whole ms, read as the missile
    lands, over the fixed 12 s: 5 ticks from 25% haste, 6 from 50%.
  - The cast owns the DoT and deals it under tag 1, which no rotation can name.
- **Ignite** takes any fire crit, a tick's too (spell_proc -11119): Living Bomb's with its glyph, Pyroblast's.
- **Arcane Blast's buff** (36032) lasts 6 s. An Arcane Barrage or Arcane Explosion cast ends it (spell_proc
  36032, cast phase) after the launch, so that cast still hits with the stacks.
  - A stack adds 175% to the cost; with Netherwind Presence and `ArcaneBlastHasteCost`, int32(175 × min(cast
    speed, 1)) in float32. The server works it out on each stack gained and on a refresh at four, so a haste
    change shows at the next Arcane Blast to land.
- **Spell Power** with `ArcaneCritRefund`: an arcane mage spell's crit gives back 15% a point of what its cast
  paid (nothing under Clearcasting), once a cast (spell_proc -35578, 8 ms), as an energize under the talent's id.
  An Arcane Missiles crit takes a sixth of the channel's base cost, a free channel's too: `SpellInfo::GetMaxTicks`
  counts no `PERIODIC_TRIGGER_SPELL` effect and gives its default 6.

## Priest

- **Penance:** Mental Agility's mask leaves it out. Its damage tick takes 0.1905 of spell power, and Searing
  Light and Twin Disciplines multiply, as the heal's four talents do. 53007 and its channel can't miss; each tick
  rolls its own hit.
- **Circle of Healing**, **Prayer of Healing** and **Prayer of Mending**'s bounce skip a pet that isn't out:
  their search finds only units in the world.
- **Silent Resolve** cuts holy and arcane threat ([threat](threat.md)), so not Mind Blast's.
- **Improved Devouring Plague**'s burst comes off the applied aura (`Aura::HandleAuraSpecificMods`), so a
  resisted cast has none. It counts the dot's `GetTotalTicks`: 8 on a new dot without `ShadowDotHaste`. A
  recast over a running one in Shadowform then counts 24000 / int32(3000 × cast speed), 10 at 25% haste:
  `Aura::RefreshTimers` puts the 24 s back before the burst, and the cast shortens it only after.

## Warlock

- **Curse of Doom** with Demonic Pact and `CurseOfDoomScaling` (the carrier 425793): max(60 s × min(cast
  speed, 1), 1 s) in whole ms, read at the cast, is the cooldown, the duration and the one tick. The tick
  crits, for 1.5: no crit damage or crit chance mod lists the curse. A cast is refused while the warlock's own
  curse is on the target; stock, a recast wipes it. A rotation compares the fight's time left to the APL value
  `APLValueWarlockCurseOfDoomLength` (60 s stock), never to a literal 60 s.
- **Improved Demonic Tactics** gives the demon int32(int32(the warlock's sheet crit %) × 10 a point / 100),
  spell crit for spell and melee for melee, reworked every 10 s from a drawn phase; a demon summoned later
  takes the current value at once (`sim/warlock/pet.go`).
