# Class mechanics

Class spells, talents and procs where the server differs from what the Classic sim did, and the sim now
follows it. Core rules (costs, threat, crit damage, procs, pets) are in [server parity](server-parity.md).

## Death knight

- **Frost Vulnerability** (Razorice's 51714) rolls spell hit.
- **Blade Barrier** refreshes on any DK cast that leaves both blood slots on cooldown, checked in the cast phase,
  after the cost and before the effects (`bladeBarrierCastPhase`, run first in `ApplyEffects` by the spells that
  skip `OnCastComplete`).

## Warrior

- **Rend:** `spell_warr_rend` adds the weapon fifth after `SpellDamageBonusDone`, so only the base takes the done
  percents.
- **Flurry**, the shaman's too: charges go on landed white hits only, and the buff lasts 15 s. A shaman's white
  crit spends a charge before the talent refreshes, unless it was the last.

## Paladin

- **Vindication** procs at 3 PPM (spell_proc -9452) on landed damaging swings, casts and `NOT_A_PROC` triggered
  spells; its 26017 (67 at one point) rolls spell hit.
- **Crusader Strike** refreshes no judgement. **Improved Devotion Aura** is improved at 3 points.
- **Glyph of Reckoning** has a server script (mod-spell-tweaks) but no client data, so no player has it: Hand of
  Reckoning never deals damage (67485), and the sim registers nothing for it.

## Rogue

- **Prey on the Weak:** a 2 s check puts on the 10 s 58670 while the rogue's health percent beats the target's,
  and only once it's gone, so it's down one check in six.
- **Hemorrhage**'s debuff goes on solo; its charges go on any landed hit on the target but poisons.
- **Honor Among Thieves** procs at the finish phase (spell_proc 51701), so crit ticks give no combo point.
- **Master Poisoner** (`SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER`) raises only its rogue's crit against a poisoned
  target, so the sim gives it to that rogue as a buff, not to the raid as a debuff.

## Druid

- **Eclipse** procs only on crits (spell_proc -48516 HitMask 2): `spell_dru_eclipse` rolls the ProcChance (×0.6
  for Wrath), then spell_proc's chance; never while either eclipse is up.
- **Vengeance** covers Wrath, Moonfire, Starfire and Starfall, not Insect Swarm.
- **Rake**'s first hit takes armor.
- **Forms:** cat and bear weapon damage is min(level, 60) × 0.85 / 1.25 × the unhasted attack time; a weapon's feral
  AP truncates to int32 in float32; Predatory Strikes gives 7/14/20% of that feral AP plus the item's own AP.
- **Starfall**'s stars (53195, `NOT_A_PROC`) proc nothing; the 53190 splash does.
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
  snares, not `MECHANIC_SLOW_ATTACK`.
- **Brain Freeze:** Fireball! (57761) lasts 15 s and is used at the cast; T10's 2-piece triggers only on that use,
  never on the timeout.
- **Enduring Winter:** a landed Frostbolt, at most every 6 s (spell_proc -44557). **T8's 4-piece** is 20%.
- **Master of Elements** refunds 10% a point of the truncated base cost on any fire or frost crit, whatever the cast
  paid; **Burnout** burns apart. Each at most once per 8 ms (spell_proc -29074, -44449), so a crit on several targets
  refunds once.
- **Blizzard**'s tick (42938) is a proc: only `CAN_PROC_FROM_PROCS` auras take it.

## Hunter

- **Ranged Weapon Specialization**, with a bow, gun or crossbow, covers every non-physical school and, of physical,
  only ranged attacks.
- **Hunter's Mark:** a weapon hit's weapon part reads the hunter's own ranged AP, and the mark adds
  int32(bonus / 14 × speed) after the weapon percent. Kill Shot's 0.4 AP reads no mark.
- **Thrill of the Hunt** returns 40% of the shot's truncated base cost, a free Lock and Load shot's too.
- **Explosive Decoy** deals nothing: its creature casts 53273 only on death, and a boss never attacks it.
- **Haste:** mod-individual-progression's aura 89507 never changes the speed, so ranged haste is just the quiver's
  or ammo pouch's, which `Hunter.Options.quiver` picks (×1.15 by default), times Thori'dal's own ×1.15 (44972).
