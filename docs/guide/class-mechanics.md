# Class mechanics

Spells, talents and procs of the death knight, warrior, paladin, rogue and hunter where the server differs
from what the Classic sim did, and the sim now follows it. The druid, shaman, mage, priest and warlock are in
[caster mechanics](caster-mechanics.md). Core rules (costs, threat, procs) are in
[server parity](server-parity.md), damage and heal amounts in [damage and healing](damage-and-healing.md),
pets and guardians in [pets](pets.md).

## Miss cost

A player's rage, energy, rune or runic power spell with an explicit unit target takes only irand(0, cost / 4)
when that target's result isn't a hit, block, absorb or reflect (`Spell::TakePower`; `missCostTaken`, rage in
tenths). A sim spell opts in by calling `IssueRefund` on that result, the druid's cat and bear strikes too.

- `SPELLMOD_SPELL_COST_REFUND_ON_FAIL` cuts the cost before the `/ 4`. It's the cost options' `Refund`: Primal
  Precision on Ferocious Bite and Rip, nothing else.
- **Quick Recovery** isn't that mod but a proc (`spell_rog_quick_recovery`): an energize of its own, 40 / 80%
  of the listed cost, truncated, on top (`EnergyCostOptions.ProcRefund`: Eviscerate, Envenom, Expose Armor,
  Rupture).
- Cleave pays by its main target's result. A full block pays in full (Hamstring).
- An area spell pays in full: `Spell::InitExplicitTargets` drops a unit target the spell's explicit mask
  doesn't name (Whirlwind, Thunder Clap, the shouts, Fan of Knives).
- So does a pet, which isn't a player: the ghoul's Claw, a hunter pet's focus.
- Runes and a runic power cost: [server parity](server-parity.md#other-measured-server-behaviour).

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
  - Only off a target that gives XP or honor (`PROC_ATTR_REQ_EXP_OR_HONOR`; `givesXPOrHonor`): above the gray
    level and without `proto.Target.no_xp`, the creature's `NO_XP` flag.
  - Improved Blood Presence adds 5% healing taken a point, in Blood Presence only.
- **Death Strike**'s heal (45470) and **Death Pact**'s (48743) have no damage class either: only the healing
  taken percents reach them (`gainHealingTakenOnly`).
- **Improved Unholy Presence** scales the rune cooldown after the grace cut, in whole ms.
- **Death Coil** and **Rune Strike** are refundable ([miss cost](#miss-cost)). Sudden Doom's Death Coil pays
  nothing.
- **Icebound Fortitude:** 30 + int32((full defense skill − 400) × 0.15) percent, at least 40 with the glyph.
- **Rune of the Stoneskin Gargoyle** (3847) is +25 defense skill, not rating.

## Warrior

- **Rend:** `spell_warr_rend` adds the weapon fifth after `SpellDamageBonusDone`, so only the base takes the done
  percents.
- **Flurry**, the shaman's too: charges go on landed white hits only, and the buff lasts 15 s. A shaman's white
  crit spends a charge before the talent refreshes, unless it was the last.
- **Titan's Grip**'s -10% (49152) needs both hands full and a two-hander in either; a shield or held item fills
  the off hand like a weapon.
- **Sweeping Strikes** copies the hit's damage before armor (`GetUnmitigatedDamage`;
  `SpellResult.ArmorReduced`). Execute's copy outside the execute phase is a whole normalized weapon roll, and
  only a Whirlwind's first hit is copied (0.5 s lockout).
- **Hamstring** (1715) is only its 15 s snare, which a mage's Torment the Weak reads (`core.SnareAuraTag`). A
  snare-immune target is immune to the whole spell: a miss, at the [miss cost](#miss-cost).

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

## Hunter

- **Ranged Weapon Specialization**, with a bow, gun or crossbow, covers every non-physical school and, of physical,
  only ranged attacks.
- **Hunter's Mark:** a weapon hit's weapon part reads the hunter's own ranged AP; the mark joins the
  creature-type bonus ([damage and healing](damage-and-healing.md#damage)). Kill Shot's 0.4 AP reads no mark,
  and its weapon part the weapon's own speed.
- **Chimera Shot - Serpent** (53353) ignores caster modifiers (no Mortal Shots, no Marked for Death) and deals
  int32(the sting's stored tick × 2, 2.8 with Glyph of Serpent Sting) × the target's damage taken as the
  sting's, truncated (`spell_hun_chimera_shot`). The stored tick is whole and keeps the done percents the
  sting went on with; Chimera Shot's refresh rereads only its AP. 53353's own hit takes a damage taken
  multiplier only below 1 (`SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS`), so a bonus counts once and a reduction
  twice. Flat damage taken (Gift of Arthas) joins the tick at the sting's dot coefficient, ahead of the taken
  percents (`Spell.PeriodicFlatDamageTaken`).
- **Hunting Party** rolls on each crit tick of Explosive Shot (53352, in spell_proc -53290's mask with
  `TRIGGERED_CAN_PROC`), never on its cast, at 33/66/100%.
- **Thrill of the Hunt** returns 40% of the shot's truncated base cost, a free Lock and Load shot's too.
- **Pin** (53548) is a root, so armor cuts its ticks. **Savage Rend** (53582) is a bleed: armor cuts the first
  hit, not the ticks (`Unit::IsDamageReducedByArmor` finds the bleed per effect only).
- **Explosive Decoy** deals nothing: its creature casts 53273 only on death, and a boss never attacks it.
- **Haste:** mod-individual-progression's aura 89507 never changes the speed, so ranged haste is just the quiver's
  or ammo pouch's, which `Hunter.Options.quiver` picks (×1.15 by default), times Thori'dal's own ×1.15 (44972).
