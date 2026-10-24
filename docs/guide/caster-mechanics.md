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
- **Rake**'s first hit takes armor. Its ticks crit with Primal Gore (mod-spell-tweaks adds Rake to 63503's
  mask; stock it's Rip and Lacerate) or the T10 4-piece.
- **Rend and Tear** gives Ferocious Bite 10% crit a rank on a bleeding target (mod-spell-tweaks; stock 5).
- **Idol of the Ravenous Beast** adds 135 to Shred's base, 304 after its 225% (mod-spell-tweaks' 60773; stock
  90, 203).
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
  `SpellFlagNaturesGrace`.
- **Resto heals** (`sim/druid/heals.go`, `lifebloom.go`, `wild_growth.go`; amounts in
  [damage and healing](damage-and-healing.md#healing)):
  - All cast in Tree of Life: the form has `SHAPESHIFT_FLAG_STANCE`, so acts as no form, and no heal's StancesNot
    lists Tree. Faerie Fire's and the balance spells' do; Faerie Fire at a friend (a healer's default target)
    isn't cast.
  - Omen of Clarity rolls on every landing, a hot's with nothing to heal too (`CheckProc` passes any druid
    spell but a shapeshift): Wild Growth once a target, Lifebloom's bloom (`CAN_PROC_FROM_PROCS`).
    Clearcasting covers every heal, not Tree of Life.
  - Balance talents reach them: Genesis +1% a point on the hots, Moonglow -3% cost on Healing Touch,
    Rejuvenation, Regrowth and Nourish, Nature's Majesty +2% crit on Healing Touch and Nourish, Nature's
    Splendor +3/6/2 s on Rejuvenation, Regrowth and Lifebloom, Naturalist -0.1 s on Healing Touch.
  - Resto talents, a point each (`sim/druid/heals.go`, `resto_talents.go`): Gift of Nature +2% on Healing Touch,
    Regrowth's heal, Nourish and the bloom (not Swiftmend) and on the hots; Improved Rejuvenation +5% on
    Rejuvenation; Empowered Touch +0.2 and +0.1 coefficient on Healing Touch and Nourish; Empowered Rejuvenation
    +4% coefficient on the hots, Regrowth's heal and the bloom; Nature's Bounty +5% crit on Regrowth and Nourish;
    Tranquil Spirit -2% cost on Healing Touch and Nourish; Subtlety -10% threat on every heal, and on Innervate at
    ranks 1 and 3 only (rank 2's mask lacks it); Gift of the Earthmother +2% cast speed (multiplying Celestial
    Focus) and -30 ms on Lifebloom's GCD before haste. Nature's Focus does nothing (no pushback).
  - In Tree of Life only: Master Shapeshifter's 48422, +2% a point healing done (aura 136); Improved Tree of Life,
    5% a point of Spirit as spell power (the sim has no healing-only power) and +67/133/200% item armor. A druid
    starting in the form takes it in `Druid.Reset`, since the spells reset after the auras.
  - **Living Seed** (`spell_dru_living_seed`): a crit of Healing Touch, Regrowth's heal, Nourish or Swiftmend has
    33% a point to put 48504 on the target (15 s, one charge, a new one replacing the old) for 30% of the heal.
    The target's next hit, a stand-in at 30 a minute (`core.HitTakenStandIn` from the seed's gain), spends it on
    48503 ([damage and healing](damage-and-healing.md#healing)). 48503 rolls Omen of Clarity (family 0, magic,
    direct heal; the druid is its proc actor through `originalCaster`); 48504 doesn't.
  - **Revitalize** (`spell_dru_revitalize`): each Rejuvenation or Wild Growth tick rolls 5% a point to give its
    target, by power type, 1% of max mana (48542), 4 rage, 8 energy or 16 runic power; the druid makes the
    energize's threat. A dummy has no power, so only the druid's own Wild Growth gains.
  - **Lifebloom:** each cast adds a stack, up to 3, and restarts the ticks; a tick heals one stack's whole
    amount × stacks. Only running out blooms (`AURA_REMOVE_BY_EXPIRE`), never a refresh or the fight's end:
    mana back first (int 28 × base mana × stacks / 100 / 2), then 33778.
  - **Wild Growth** (`spell_dru_wild_growth`): past 5, the living raid units and pets lowest in health
    percent, a tie to raid order.
  - **Tree of Life** (33891, 13% base mana less Natural Shapeshifter): its 34123 gives every raid unit +6%
    healing taken (effect 65, a raid area aura). Tree of Life Passive (5420, -20% cost on Rejuvenation, Regrowth, Lifebloom and Wild Growth)
    has no Stances, so holds in any form. A Resto druid with the talent starts in the form.
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
- **Elune's Wrath** (64823, the Balance T8 4-piece) makes Starfire instant and goes with the Starfire it made
  instant, not one already casting as it came up (`PROC_ATTR_REQ_SPELLMOD`; `sim/druid/insect_swarm.go`).
- **Survival Instincts** is [bonus health](class-mechanics.md#bonus-health).
- The Feral Tank's automatic rotation is its default APL, whose `autocastOtherCooldowns` casts Survival
  Instincts, Frenzied Regeneration and Barkskin.

## Shaman

- **Lightning Overload** (`spell_sha_lightning_overload`; `sim/shaman/lightning_overload.go`) casts 45284 or
  45297 at the hit's target with base points int32(dealt [/ 2 on a crit]) / 2, resist and crit included; the
  bolt's level scaling and die add 1 (45284) or 4 to 16 (45297). It rolls its own hit and crit and takes again:
  - its spell power share, 0.0625 (45284) or 0.2855 (45297) plus Shamanism, at the level penalty (level 1:
    0.0252; level 32: 0.475);
  - the spell mods (Concussion, Glyph of Lightning Bolt, Skyshatter 4-piece, crit damage) and the target's
    taken percents;
  - of done percents only the shaman's own shaman-family ones (Elemental Oath's):
    `SPELL_ATTR6_IGNORE_CASTER_DAMAGE_MODIFIERS` (`SpellInfoCorrections`) drops the raid's 3%, Tricks and
    creature-type bonuses.

  It goes after the hit's Elemental Focus proc (auras proc in spell id order, 16164 first), so a crit's
  Clearcasting counts for its own overload. Chain Lightning's chance is the aura's 11% a rank × the script's
  33%; 45297 jumps like the cast from its hit's target.
- **Call of Thunder** (mod-spell-tweaks): +70% crit damage bonus on Lightning Bolt and Chain Lightning,
  overloads included, adding to Elemental Fury's.
- **Lava Lash** takes the whole off hand and, a melee-class spell, crits ×2, outside Elemental Fury's mask.
- **Enhancing Totems:** Flametongue Totem's 58655 is a raid area aura, so its int32(144 × 1.15) = 165 reaches every
  raid member and pet.
- **Improved Healing Wave** takes 0.1 s a point off Healing Wave.
- **Chain Heal** picks every jump before its first heal lands (`chainHealJumps`; `Spell::SearchChainTargets`):
  the enabled, [living](damage-and-healing.md#healing) unit not yet healed by the chain with the largest whole
  health deficit over 0, a tie to the first in raid order. A full-health unit never qualifies, so a full raid
  takes one heal a cast. With no positions, everyone is in range and sight.
  - Nothing damages the healer sim's target dummies, so each counts as missing 1 health (`standInDeficit`),
    a stand-in for a hurt raid: a real deficit goes first, and the caster at full health is skipped.
  - Every target's heal and crit is rolled before the first one lands (`Spell::DoAllEffectOnLaunchTarget`),
    so Tidal Force's charges don't thin the later targets' crit chance.
  - `spell_sha_chain_heal` looks for the caster's Riptide on the first target only, consumes it there, and
    raises every heal of the cast by 25%.
- **Healing Way** is +8/16/25% on Healing Wave, a damage spell mod that multiplies with Purification.
- **Tidal Waves**' buff (53390) ends after 15 s, charges left or not.
- **Mana Tide:** the shaman's own cast restores its whole party, so core's external Mana Tides
  (`PartyBuffs.ManaTideTotems`, which `AddPartyBuffs` counts the shaman into for anyone else's sim) leave out
  the party's own casters (`core.ManaTideCaster`).
- **Tidal Force** (55198, 3 min, no GCD) puts 55166 on the shaman: 3 stacks of +20% crit chance on Healing Wave,
  Lesser Healing Wave and Chain Heal for 20 s. Each crit of those spends a stack (`spell_proc`).
- **Earth Shield**'s charges come off a stand-in hit taken at the page's `earth_shield_p_p_m` a minute
  (`core.HitTakenStandIn`, first a period after the cast), 3.5 s apart (379's cooldown on the target). Riptide
  and Earth Shield exist only with their talents.
- **Glyph of Healing Wave** (`spell_sha_glyph_of_healing_wave`) casts 55533 on the shaman for
  `int32(heal × 20%)` of each Healing Wave on someone else: the healing done percents and no spell power.
  **Glyph of Riptide** adds 6 s (2 ticks). **Glyph of Mana Tide** makes each tick 7%. **Glyph of Water
  Mastery** adds 30% to Water Shield's 100 mana per 5 s. **Glyph of Water Shield** (an orb) does nothing:
  orbs need hits taken.
- **Resto sets:** T7 (Earthshatter Regalia) 2-piece adds 10% to Water Shield (10 mana per 5 s and 10% of the
  orb, added to Improved Shields'), 4-piece +5% Healing Wave and Chain Heal. T8 takes 1 s off Riptide's
  cooldown and 0.2 s off Chain Heal's cast. T9 (Thrall's/Nobundo's Garb) is +20% Riptide, direct and periodic,
  and +5% Chain Heal crit. T10 (Frost Witch's Garb) 2-piece: a Riptide hit gives 20% cast speed (70806) to the
  next cast, 10 s; 4-piece: a Chain Heal crit puts 70809 on its target, 3 ticks of `int32(int32(heal × 25%) / 3)`
  with the healing done percents again (`spell_sha_t10_restoration_4p_bonus`), plus `int32(old × ticks left / 3)`
  of one already there, read at the crit; it's [munched](spell-data-and-timing.md#update-timing) like Ignite.
- **Resto totems:** Misery takes 79 off Healing Wave's base cost; Relentless and Wrathful Totems of the Third
  Wind add 404 and 459 healing power to Lesser Healing Wave; Calming Tides rolls 70% (8 s) on a Chain Heal
  cast, before its heals, for 234 spell power, 15 s; Surging Sea gives 85 spell power a Riptide, 15 s, up to 3.
- **Improved Water Shield** rolls on a heal's crit at 33/66/100% × 60% for Lesser Healing Wave, 30% for Chain
  Heal, 100% otherwise (`impWaterShieldChance`; `spell_sha_imp_water_shield`), and gives mana only with Water
  Shield up, using no orb.
- **Resto page** (`ui/restoration_shaman/apls/default.apl.json`): Call of the Elements and Earth Shield before the
  pull, then Earth Shield when it's gone, Tidal Force, Riptide on cooldown, Healing Wave at 2 Tidal Waves
  stacks, Chain Heal; Mana Tide and Nature's Swiftness go out as major cooldowns. The page's Earth Shield rate
  is 30 hits a minute (a boss swing every 2 s).
  - Every phase's gear runs dry inside the healer pages' 300 s default fight (P1 at ~130 s, P4 at ~181 s).
    Without its Healing Wave line the APL heals 10–27% more and still runs dry at every phase (P1 at ~255 s). Improved Water Shield's crit procs are the largest mana source.
  - Spirit gives mana only outside the five-second rule, and even out of mana the shaman casts again as soon as
    in-combat regen pays for a spell, so Spirit measures about 0 at every phase and fight length.
  - Val'anyr's shields count as healing, so it beats the 893 spell power scepter in HPS.
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
- **Combustion** (`spell_mage_combustion`): each landed fire hit queues a stack for the mage's next tick, a 1 ms
  event (`sim.QueueSpellEvent`), dropped if Combustion has gone, so a cast's hits roll without each other's
  stacks and a hardcast completing on that tick goes first. Only a crit takes a charge, at once. Stacks cap at
  28682's 10.
- **Blizzard**'s tick (42938) is a proc: only `CAN_PROC_FROM_PROCS` auras take it.
- **Pyroblast's DoT** with Empowered Fire:
  - Its ticks crit whatever the switches (aura 286 on the talent is `spell_dbc` data), at the cast's crit
    chance, snapshotted at the landing. A crit tick feeds Ignite; Hot Streak, Master of Elements, Combustion
    and Burnout take no tick.
  - With `PyroblastEmpoweredFire` the tick is min(3 s × cast speed, 3 s) in whole ms, read as the missile
    lands, over the fixed 12 s: 5 ticks from 25% haste, 6 from 50%.
  - The cast owns the DoT and deals it under tag 1, which no rotation can name.
- **Ignite** takes any fire crit, a tick's too (spell_proc -11119): Living Bomb's with its glyph, Pyroblast's.
- **Arcane Empowerment** adds 5 a point to Arcane Blast's and the Arcane Missiles missile's (42845) spell
  power coefficient, in hundredths (mod-spell-tweaks' `spell_dbc` rows; stock 3).
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
  - 53007 is a channel with no duration: it casts from its `SpellEvent` on the press's world tick, and the
    script's channel (53003, or 52988 at a friend) from its own on the next, firing the first bolt
    (`penanceChannelStart`, `sim.QueueSpellEvent`). 53007's mana, the amount worked out at the press, and its
    cooldown go on that tick too, the GCD at the press (`penancePress`, `penanceCost`).
  - Each bolt, 53000 or the heal's 52985, is a missile at 40 yd/s, rolled at the tick and dealt after its
    flight (`sim/priest/penance.go`; a KeepSim allowance on 53007's speed), a heal's to a friend or the priest
    as [missiles](missiles.md#flight-and-reach) give it.
- **Healing Priest APLs** (`ui/healing_priest/apls`) cast on every GCD with no `spellCpm` caps; the mana pool
  limits them. In the healer pages' 300 s default fight holy runs out of mana at every phase, disc at preraid
  to P3 but not at P4 and P5 (the sim estimates ~373 and ~328 s). A stat that only matters while mana-bound
  (Spirit, MP5) measures near 0 once the fight ends first, and haste about 0 or below while the priest is
  mana-bound.
- **Twisted Faith** at 5/5 gives 53% of Spirit as spell power, damage and healing (mod-spell-tweaks'
  `spell_dbc` row for 51167; stock 20%). Ranks 1–4 stay 4% a rank.
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
- **Crit damage** of Shadow Bolt, Incinerate and Soul Fire: mod-spell-tweaks adds Nemesis' 10% a point, and
  on Shadow Bolt Empowered Corruption's 14%, to Ruin's 20% (`SPELLMOD_CRIT_DAMAGE_BONUS`, percents add;
  `nukeCritDamageBonus`).
- **Pyroclasm**'s buff is 7/13/20% fire and shadow damage (mod-spell-tweaks; stock 2/4/6%).
- **Demonic Pact** procs off the demon's crits at most every 20 s (spell_proc 53646, the passive demons learn
  from their family's skill line; `ServerProcFor`). The entry's chance is 0; the talent's
  `SPELLMOD_CHANCE_OF_SUCCESS` +100 makes it certain.
