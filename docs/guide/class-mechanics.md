# Class mechanics

Class spells, talents and procs where the server differs from what the Classic sim did, and the sim now
follows it. Core rules (costs, threat, crit damage, procs) are in [server parity](server-parity.md), pets and
guardians in [pets](pets.md).

## Death knight

- **Frost Vulnerability** (Razorice's 51714) rolls spell hit.
- **Blade Barrier** refreshes on any DK cast that leaves both blood slots on cooldown, checked in the cast phase,
  after the cost and before the effects (`bladeBarrierCastPhase`, run first in `ApplyEffects` by the spells that
  skip `OnCastComplete`).
- **Death runes:** each `spell_dk_death_rune` proc restarts its talent's 30 s periodic, which then reverts every
  rune that talent turned; Blood of the North and Reaping share one. Chances are spell_proc's (30/60/100%,
  33/66/100%). A Threat of Thassarian off-hand strike rerolls a partial rank; the 100 ms cooldown starts only on
  a proc.
- **Blood Tap** readies a rune only for a blood-slot death rune on cooldown, or with both plain blood runes down;
  a death rune talent takes over the tapped slot.
- **Dirge** covers Obliterate.
- **Blood Presence**'s heal (63611: 4%, Improved Blood Presence 2% a point in the other presences) comes from
  swings, own casts, dot ticks, `NOT_A_PROC` triggered spells and an on-use item's damage, which
  `Player::CastItemUseSpell` casts untriggered (`onUseItemDamage`: the explosives, Pyro Rocket, Gnomish
  Lightning Generator; the sapper's hit on the DK too).
  - It's int32(int32(damage × percent) × the healing taken percents): 50475 has no damage class, so no healing
    done percent reaches it, Strength of Wrynn's included (`Unit.HealingTakenPctMultiplier`).
  - Only off a target that gives XP or honor (`PROC_ATTR_REQ_EXP_OR_HONOR`). The sim checks the gray level
    (`givesXPOrHonor`), not the `NO_XP` flag, so it heals off a `NO_XP` creature (the simval dummies) where
    the server doesn't.
  - Improved Blood Presence adds 5% healing taken a point, in Blood Presence only.
- **Improved Unholy Presence** scales the rune cooldown after the grace cut, in whole ms.
- **Death Coil** and **Rune Strike** are refundable: a miss takes irand(0, cost / 4). Sudden Doom's Death Coil
  pays nothing.
- **Icebound Fortitude:** 30 + int32((full defense skill − 400) × 0.15) percent, at least 40 with the glyph.
- **Rune of the Stoneskin Gargoyle** (3847) is +25 defense skill, not rating.

## Warrior

- **Rend:** `spell_warr_rend` adds the weapon fifth after `SpellDamageBonusDone`, so only the base takes the done
  percents.
- **Flurry**, the shaman's too: charges go on landed white hits only, and the buff lasts 15 s. A shaman's white
  crit spends a charge before the talent refreshes, unless it was the last.
- **Titan's Grip**'s -10% (49152) needs both hands full and a two-hander in either; a shield or held item fills
  the off hand like a weapon.

## Paladin

- **Vindication** procs at 3 PPM (spell_proc -9452) on landed damaging swings, casts and `NOT_A_PROC` triggered
  spells; its 26017 (67 at one point) rolls spell hit.
- **Crusader Strike** refreshes no judgement.
- **Seal of Vengeance**'s DoT stacks off white hits and Hammer of the Righteous; off Crusader Strike and Divine
  Storm only with `SpellTweaks.Enable` (`spell_tweaks_seal_dot_on_strikes` has no switch of its own).
- **Improved Devotion Aura:** 3 points is the raid buff's improved state; 1–2 give every raider and pet under
  the best paladin's aura +204 / +409 armor (`improveDevotionAura`). Its 63514 adds 2% healing taken a point.
- **Crusade:** its first percent is aura 79 on all damage (`DamageDealtMultiplier`), its second DONE_VERSUS on
  humanoids, demons, undead and elementals, per target.
- **Glyph of Reckoning** has a server script (mod-spell-tweaks) but no client data, so no player has it: Hand of
  Reckoning never deals damage (67485), and the sim registers nothing for it.

## Rogue

- **Prey on the Weak:** a 2 s check puts on the 10 s 58670 while the rogue's health percent beats the target's,
  and only once it's gone, so it's down one check in six.
- **Hemorrhage**'s debuff goes on solo; its charges go on any landed hit on the target but poisons.
- **Honor Among Thieves** procs at the finish phase (spell_proc 51701), one roll a cast, so crit ticks give no
  combo point. Fan of Knives rolls once per hand; Mutilate's hands, poisons and Killing Spree are triggered
  casts of their own, each rolling (`CAN_PROC_FROM_PROCS`).
- **Master Poisoner** (`SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER`) raises only its rogue's crit against a poisoned
  target, so the sim gives it to that rogue as a buff, not to the raid as a debuff.
- **Glyph of Backstab:** a landed Backstab adds 2 s to Rupture (`extendRupture`).
  - With `GlyphOfBackstabTicks`: 6 s a Rupture cast, to the max duration too, so the seconds tick, at the
    interval Rupture went on with (haste isn't read again). 5 points: 11 ticks unhasted.
  - Stock: only the time left grows, while the aura would end within 5 s of its apply time plus max duration
    (three Backstabs), and ticks stop at max duration / amplitude. 5 points: 8 ticks, aura off at 22 s.
    Not modelled: a recast over a live Rupture keeps the server aura's first apply time, so it gets fewer or
    no extensions.

## Druid

- **Eclipse** procs only on crits (spell_proc -48516 HitMask 2): `spell_dru_eclipse` rolls the ProcChance (×0.6
  for Wrath), then spell_proc's chance; never while either eclipse is up.
- **Vengeance** covers Wrath, Moonfire, Starfire and Starfall, not Insect Swarm.
- **Rake**'s first hit takes armor.
- **Forms:** cat and bear weapon damage is min(level, 60) × 0.85 / 1.25 × the unhasted attack time; a weapon's feral
  AP truncates to int32 in float32; Predatory Strikes gives 7/14/20% of that feral AP plus the item's own AP.
- **Starfall** (53201) is an aura on the druid and can't miss. Its stars (53195) and splash (53190) roll Omen of
  Clarity, as do Hurricane's cast and ticks (measured live, 8.75%). The stars need line of sight to the target's
  centre.
- **Omen of Clarity** rolls 3.5 PPM over the spell's cast time entry, at least 1.5 s: 8.75% an instant, 11.67%
  Wrath (2 s). Any landed magic-class spell outside the druid's family that deals direct damage or heals and
  gives no power rolls it too, an item's proc included (`rollsOmenOffFamily`): Extract of Necromantic Power's
  bolt at 12.83%. Nature's Grace's 16886 and Survival Instincts roll nothing.
- **Nature's Grace** rolls its chance in the cast phase and again in the hit phase (spell_proc -16880): a crit
  Wrath gives 16886 at its launch and at its hit, a cast spell without a missile rolls both at its hit, a
  triggered one (Starfall's stars) the hit phase only (`rollNaturesGrace`).
- **Hurricane**'s cast is a hit without damage on the druid's side: one Illustration of the Dragon Soul stack,
  none from its ticks.
- **Faerie Fire (Feral):** a landed 16857 rolls Omen's 3.5 PPM over the 1.5 s an instant counts as (8.75%), and
  so does Bear Form's triggered 60089 (`CAN_PROC_FROM_PROCS`). With `OmenClarityFaerieFire` the script casts
  Clearcasting (16870) itself, and that roll still runs: only it gives the Balance T10 2-piece proc (70721).
- **Force of Nature** casts 33831, inside Clearcasting's mask.
- **Nature's Swiftness** makes instant what its aura's class mask covers (`Spell.AffectedByAuraEffect`): heals and
  Rebirth, not Starfire.

## Shaman

- **Lava Lash** takes the whole off hand and, a melee-class spell, crits ×2, outside Elemental Fury's mask.
- **Enhancing Totems:** Flametongue Totem's 58655 is a raid area aura, so its int32(144 × 1.15) = 165 reaches every
  raid member and pet.
- **Improved Healing Wave** takes 0.1 s a point off Healing Wave.

## Mage

- **Torment the Weak** counts only against a snared or attack-slowed target, whoever did it: raid bosses and
  the dummy are immune to snares ([mechanic immunity](server-parity.md#other-measured-server-behaviour)), not
  `MECHANIC_SLOW_ATTACK`. A landed Frostbolt (not under Glyph of Frostbolt) or Frostfire Bolt (9 s), or Chilled
  (1.5 s), slows any other target, Permafrost included; so do a Mirror Image's Frostbolt, Blast Wave (6 s) and
  Dragon's Breath (5 s, gone at the first direct damaging hit: one proc charge). Frostbite is a root.
  - Another unit's snare counts through an aura tagged `core.SnareAuraTag`: another mage's chill, Frost Shock
    (8 s), Mind Flay. Hamstring and Slow have no sim spell.
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

## Hunter

- **Ranged Weapon Specialization**, with a bow, gun or crossbow, covers every non-physical school and, of physical,
  only ranged attacks.
- **Hunter's Mark:** a weapon hit's weapon part reads the hunter's own ranged AP; the mark joins the
  creature-type bonus ([server parity](server-parity.md)). Kill Shot's 0.4 AP reads no mark, and its weapon part
  the weapon's own speed.
- **Chimera Shot - Serpent** (53353) ignores caster modifiers (no Mortal Shots, no Marked for Death) and deals
  int32(the sting's stored tick × 2, 2.8 with Glyph of Serpent Sting) × the target's damage taken as the
  sting's, truncated (`spell_hun_chimera_shot`). The stored tick is whole and keeps the done percents the
  sting went on with; Chimera Shot's refresh rereads only its AP. 53353's own hit takes a damage taken
  multiplier only below 1 (`SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS`), so a bonus counts once and a reduction
  twice. Not modelled: the flat taken part (Gift of Arthas).
- **Hunting Party** rolls on each crit tick of Explosive Shot (53352, in spell_proc -53290's mask with
  `TRIGGERED_CAN_PROC`), never on its cast, at 33/66/100%.
- **Thrill of the Hunt** returns 40% of the shot's truncated base cost, a free Lock and Load shot's too.
- **Pin** (53548) is a root, so armor cuts its ticks. **Savage Rend** (53582) is a bleed: armor cuts the first
  hit, not the ticks (`Unit::IsDamageReducedByArmor` finds the bleed per effect only).
- **Explosive Decoy** deals nothing: its creature casts 53273 only on death, and a boss never attacks it.
- **Haste:** mod-individual-progression's aura 89507 never changes the speed, so ranged haste is just the quiver's
  or ammo pouch's, which `Hunter.Options.quiver` picks (×1.15 by default), times Thori'dal's own ×1.15 (44972).
