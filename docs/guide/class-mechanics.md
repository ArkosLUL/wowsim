# Class mechanics

Class spells, talents and procs where the server differs from what the Classic sim did, and the sim now
follows it. Core rules (costs, threat, crit damage, procs, pets) are in [server parity](server-parity.md).

## Death knight

- **Frost Vulnerability** (Razorice's 51714) rolls spell hit.
- **Blade Barrier** refreshes on any DK cast that leaves both blood slots on cooldown, checked in the cast phase,
  after the cost and before the effects (`bladeBarrierCastPhase`, run first in `ApplyEffects` by the spells that
  skip `OnCastComplete`).
- **Death runes:** each `spell_dk_death_rune` proc restarts its talent's 30 s periodic, which then reverts every
  rune that talent turned; Blood of the North and Reaping share one. Chances are spell_proc's (30/60/100%,
  33/66/100%).
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

## Paladin

- **Vindication** procs at 3 PPM (spell_proc -9452) on landed damaging swings, casts and `NOT_A_PROC` triggered
  spells; its 26017 (67 at one point) rolls spell hit.
- **Crusader Strike** refreshes no judgement.
- **Improved Devotion Aura:** 3 points is the raid buff's improved state; 1–2 give every raider and pet under
  the best paladin's aura +204 / +409 armor (`improveDevotionAura`). RaidStats shows them unimproved.
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

## Druid

- **Eclipse** procs only on crits (spell_proc -48516 HitMask 2): `spell_dru_eclipse` rolls the ProcChance (×0.6
  for Wrath), then spell_proc's chance; never while either eclipse is up.
- **Vengeance** covers Wrath, Moonfire, Starfire and Starfall, not Insect Swarm.
- **Rake**'s first hit takes armor.
- **Forms:** cat and bear weapon damage is min(level, 60) × 0.85 / 1.25 × the unhasted attack time; a weapon's feral
  AP truncates to int32 in float32; Predatory Strikes gives 7/14/20% of that feral AP plus the item's own AP.
- **Starfall** (53201) is an aura on the druid and can't miss. Its stars (53195, `NOT_A_PROC`) proc nothing; the
  53190 splash does, but not Omen of Clarity (measured live).
- **Force of Nature** casts 33831, inside Clearcasting's mask.
- **Nature's Swiftness** makes instant what its aura's class mask covers (`Spell.AffectedByAuraEffect`): heals and
  Rebirth, not Starfire.

## Shaman

- **Lava Lash** takes the whole off hand and, a melee-class spell, crits ×2, outside Elemental Fury's mask.
- **Enhancing Totems:** Flametongue Totem's 58655 is a raid area aura, so its int32(144 × 1.15) = 165 reaches every
  raid member and pet.
- **Improved Healing Wave** takes 0.1 s a point off Healing Wave.

## Mage

- **Torment the Weak** counts only against a snared or attack-slowed target: raid bosses and the dummy are immune to
  snares, not `MECHANIC_SLOW_ATTACK`. A landed Frostbolt (not under Glyph of Frostbolt) or Frostfire Bolt (9 s),
  or Chilled (1.5 s), slows any other target, Permafrost included.
- **Brain Freeze:** Fireball! (57761) lasts 15 s and is used at the cast.
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

## Hunter

- **Ranged Weapon Specialization**, with a bow, gun or crossbow, covers every non-physical school and, of physical,
  only ranged attacks.
- **Hunter's Mark:** a weapon hit's weapon part reads the hunter's own ranged AP; the mark joins the
  creature-type bonus ([server parity](server-parity.md)). Kill Shot's 0.4 AP reads no mark, and its weapon part
  the weapon's own speed.
- **Thrill of the Hunt** returns 40% of the shot's truncated base cost, a free Lock and Load shot's too.
- **Explosive Decoy** deals nothing: its creature casts 53273 only on death, and a boss never attacks it.
- **Haste:** mod-individual-progression's aura 89507 never changes the speed, so ranged haste is just the quiver's
  or ammo pouch's, which `Hunter.Options.quiver` picks (×1.15 by default), times Thori'dal's own ×1.15 (44972).
