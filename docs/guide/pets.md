# Pets and guardians

How a player's summons follow the server: stats, timers, the buffs and casts that reach them, where they
stand, and the guardians' scripted AIs. Declaring their spells: [spell data and timing](spell-data-and-timing.md).
The files: [sim architecture](sim-architecture.md#classes-items-encounters).

## Stats

- Pet hit is floored to a whole percent. Pet scaling comes from the server's scripts. A summon starts at full
  health, its owner's stamina included.
- Create stats are the `pet_levelstats` row, else `Guardian::InitStatsForLevel`'s 22/22/25/28/27 and 50 armor
  a level; the scaling auras add a truncated percent of the owner's (`PetScalingPct`).
- Health is the create health plus `Guardian::UpdateMaxHealth`'s amount a point of stamina over the create
  value (`Pet.SetCreateHealth`): 10, but Imp 8.4, Water Elemental 7.5, Succubus 9.1, Felhunter 9.5, Felguard 11,
  Bloodworm 1.
- A pet summoned again is a new creature: `Pet.Disable` takes the inherited stats off every pet, so the next
  summon gets them once.
- A pet crits 5% plus crit auras, none from agility or intellect; magic spells crit ×1.5.
- A swing off the physical school (Fire Elemental, Shadowfiend) rolls a whole number between the truncated
  ends of (weapon + AP / 14) × attack time (`Creature::CalculateMinMaxDamage`, `Unit::CalculateDamage`;
  `rollCreatureRangeOffPhysical`). The Fire Elemental's Fire Shield rolls spell hit.
- The DK's summons carry scaling aura 67561, not 61017: no melee hit. Risen ghouls, not the army, take the
  owner's ArP (`Pet.HitScaling`, `Pet.RisenGhoul`).

## Timers and regen

- The hit (3 s), haste, scaling, focus, energy and mana timers tick on the pet's map update, counted from the
  update before its first. A pet out since before the pull starts at a drawn point of its cycles and ticks
  through the prepull. They're queued actions, not auras, so the aura expiry order doesn't move them.
- The updates differ in length ([update timing](spell-data-and-timing.md#update-timing)), so a pet's clock
  finds the update before with `sim.ServerTickBefore` and the one after with `Pet.UpdateAfter`, never a fixed
  step.
- A timed summon (`Pet.EnableWithTimeout`) leaves on its own update, the first one a duration past the update
  before its first, behind everything it does then (`TempSummon::Update`; `petUnsummonPriority`): a cast or AI
  event due on its last update still happens. Summoned again while still out, it takes the new time. On exact
  timing its time is up ahead of a cast due at that moment.
- Unsummoning kills a missile still in the air (`Pet.MissileLost`): a timed Water Elemental's last Waterbolt, a
  Mirror Image's last Frostbolt.
- Real pets rescale every 2 s, Spirit Wolves every 1 s, other guardians once at the summon. The Rune Weapon's
  attack power and crit follow the DK's on every change while it's out (`Pet.EnableDynamicStats`), its damage
  percents at each spell copy ([guardian AIs](#guardian-ais)).
- On one update: focus ahead of the pet's AI, the scaling auras, Replenishment, then mana and energy
  (`petRegenPriority`). A full bar takes no tick.
  - Focus: 24 × Bestial Discipline, truncated, every 4 s. Energy (the DK's ghouls,
    `EnableCreatureEnergyBar`): 20 every 2 s (`sim/core/focus.go`).
  - Mana, every 2 s: trunc(spirit/5 + 17); under the five second rule spirit/8 + 17 for the Imp and the Water
    Elemental, nothing for the rest. Two fifths of the whole MP5 come on top either way
    (`Pet.CreatureManaRegen`; `Creature::Regenerate`).

## Buffs and casts

- Summons with a scaling script (Infernal, treants, Shadowfiend, Spirit Wolves, Fire Elemental, Water
  Elemental guardian, Mirror Image) are immune to their scaling aura types, so buffs of those types add nothing;
  the DK's ghoul, gargoyle and army only to stats and AP (`Pet.BuffImmunity`).
- Guardians take party and raid auras like pets (`AnyGroupedUnitInObjectRangeCheck`), but an uncontrollable
  one (`Pet.Uncontrollable`: SummonProperties with neither a pet's type nor category) never gets a party or
  raid cast (`TARGET_CHECK_PARTY`/`RAID`), Replenishment and Bloodlust included: Bloodworms, Mirror Images,
  the Greater Fire Elemental, the DK's gargoyle, army and Raise Dead guardian (not Master of Ghouls' pet).
  The Water Elemental guardian has a pet bar and takes them.
- Wrath of Air is a raid area aura (effect 65), so it reaches every pet and guardian but a haste carrier, whose
  script makes it immune to positive `MOD_CASTING_SPEED_NOT_STACK`: Firebolt and Waterbolt take 2380 ms, a
  Mirror Image's Frostbolt 2857.
- The Rune Weapon takes no buffs (`Pet.OwnerBuffsOnly`): its hits read only the DK's attack power, crit and
  damage mods (`Unit::MeleeDamageBonusDone`, `SpellDamageBonusDone`, Spell.cpp's crit).
- **A pet that isn't out** takes no temporary aura: nothing can be cast on a creature that isn't in the world,
  so `Aura.Activate` returns for it and a summon starts without what was cast while it was away. A permanent
  aura stands for a buff that's always up and stays on. A caller that needs the aura on asks
  `pet.IsEnabled()` first (Cobra Strikes' stacks). A heal that picks a raid unit skips it too: Althor's Abacus,
  Chain Heal's bounces, Prayer of Healing, Circle of Healing, Prayer of Mending.
- Haste carriers (the hunter pet, the DK's ghouls, gargoyle and army, Spirit Wolves; `Unit.HasteCarrier`)
  ignore direct haste and slows, Bloodlust's haste included but not its scale (`bloodlustScaleAura`: the
  immunity is per effect), and take the owner's attack speed as whole percent
  (`Pet.OwnerHasteSource`): at the summon, then every 2 s for a pet and for Spirit Wolves
  (`Pet.OwnerHasteTicksOnGuardian`), never again for the DK's guardians.

## Reach and position

- A summon's reach is `Unit.BaseCombatReach` (0 means 1.5, a pet's) × its scale: a guardian's is its
  model's `creature_model_info.CombatReach`, a 0 there loaded as 1.5 (the timed Water Elemental's 0.85, model
  525). How a reach moves a missile: [spell data and timing](spell-data-and-timing.md#server-data).
- The Imp and the Water Elemental cast from their follow spot (`Pet.FollowSpotDistance`): their reach plus
  `PET_FOLLOW_DIST` 2 off the owner's left, him facing the target; PetAI's chase leaves a caster there while
  the target is in range. The Gargoyle casts from its summon spot, 3 yd off the DK's front left
  (`gargoyleSpotDistance`), the Rune Weapon from the DK's.
- A melee pet's missiles wait the 5 yd floor: its chase stops at both reaches, so on the server they fly at
  most the target's reach, and a pet's scale (a hunter pet's family) changes nothing there.
- Not modelled: the Rune Weapon's run-in from 2 yd in front of the target; a timed Water Elemental summoned
  while something attacks the mage casting from its spawn spot; a caster pet moving when its owner stands
  past its spell's range.

## Guardian AIs

A scripted AI casts off event timers on the guardian's own updates, counted from the update before it engages
(`Pet.AIClockStart`), with no GCD. A script cast starts no creature cooldown (`Creature::AddSpellCooldown` is
PetAI's and the pet bar's), so its DBC recovery time never runs. `TestPetsCastsRecords` replays recorded
Fire Elemental lives and Mirror Image summons, `pets_casts.jsonl`, against both AIs
([simval README](../../tools/simval/README.md#damage-probe)).

- **Greater Fire Elemental** (`npc_pet_shaman_fire_elemental`; `FireElemental.runEvents`): Fire Nova 5–20 s
  in, then every 8–15 s; Fire Blast (57984) 5–20 s in, then every 4–8 s, in whole ms.
  - Fire Nova is magic class, so its 2 s cast scales with cast speed: 1904 ms under Wrath of Air.
  - The AI casts with `CastSpell`, not `DoCast`: a Fire Blast during Fire Nova's cast breaks it
    (`Pet.InterruptCast`), about a third of the Novas.
  - Events run ahead of the swing: a cast on the update a swing is due takes it, a Blast by restarting the
    timer, a Nova by holding it. The swing after a landed Nova comes an update short
    ([spell data and timing](spell-data-and-timing.md#update-timing)).
  - Out before the pull, it engages at the pull.
- **Mirror Images** (`npc_pet_mage_mirror_image`; `sim/mage/mirror_image.go`): a 1.2 s wait, then one event an
  update, Frostbolt put back 2.5 s and Fire Blast 6.5 s, dead at 29.5 s.
  - Summoned before the pull, they open on either spell, evenly (`CasterAI`): each image rolls its own on the
    server, the sim one for all three, with the same mean.
  - Summoned into a fight, they're engaged at the summon with no victim to open on: a Frostbolt an update
    after the wait, and 3 Fire Blasts a life in range.
  - The death runs in the image's event pass (`DeathEvent`): behind a cast landing on that update, ahead of its
    AI, which starts nothing more. After a Frostbolt opener ahead of the pull the ninth Frostbolt ends within
    an update of it, so the update lengths decide whether it goes off.
  - The corpse goes at the first update 30 s in and takes a Frostbolt in the air with it.
  - An image comes up beside its mage and casts from where it stands (`MirrorImage.closeIn`). Its ranges add
    both reaches: its own 1.5, every mage race's models' (`mirrorImageReach`), and the
    [target's](spell-data-and-timing.md#server-data). The yards below are at the boss dummy's 2.5; another
    target reach moves each by the difference (`targetReachOver`).
    - Fire Blast from 24 yd (20 + reaches) on fails at its start (`Spell::CheckRange`), and its event is put
      back anyway.
    - On an update it starts with no cast going, past 19 yd (15 + reaches), the chase runs it at 1.14286 × 7
      yd/s (`ChaseMovementGenerator`): from 19.5 on it stops on 19, from nearer on 17.5 (15 + the target's
      reach). The update after a cast starts still moves it: `Unit::Update` runs the spline before the chase
      stops it for the cast.
- **Nibelung's Val'kyr** (`npc_pet_gen_valkyr_guardian`; `sim/common/wotlk/nibelung.go`) casts Smite where a
  swing would come (`DoSpellAttackIfReady`), so its attack time paces it, not a GCD: one on its first update,
  then on the first update an attack time after the last.
  - The attack time is 2 s over its melee haste, in whole ms (`Unit::resetAttackTimer`;
    `ValkyrPet.attackTime`): Windfury Totem's and Improved Icy Talons' raid area auras, Swift Retribution and
    Improved Moonkin Form reach it. That gives 15 Smites in its 30 s, 18 under 20% (1666 ms), 19 with Swift
    Retribution on top (1618 ms). It's uncontrollable, so Bloodlust doesn't.
  - `NibelungAverageCasts` × the attack time is its life.
- **Dancing Rune Weapon** (`npc_pet_dk_dancing_rune_weapon`, a `NullCreatureAI`) never swings on the server:
  `spell_dk_dancing_rune_weapon` copies each white swing the DK lands, either hand, extra attacks too, as a
  fresh roll of his own swing on his table, floored half, dealt by the weapon with no procs
  (`RuneWeaponPet.swingCopies`; `registerDrwOwnerSwing`, never dealt). Rune Strike copies as a strike. It's
  uncontrollable (SummonProperties 208), so no Bloodlust.
  - A spell copy (`castCopy`) takes the DK's damage percents as they stand, halved: his school ones on magic,
    only the halving on physical, as a strike rolls his own main hand with his percents inside
    (`DrwWeaponDamage`), normalized for Blood, Heart, Death and Plague Strike, not Rune Strike.
  - After each spell copy the DK's reach is 0.01 until his scale next changes (`Unit.SetCombatReach`), so his
    missiles fly the whole distance (Death Coil).
  - It makes no threat: `UNIT_FLAG_NON_ATTACKABLE` keeps its threat ref on the target offline, and 49028's
    redirect to the DK never runs (`SpellInfoCorrections` zeroes the effect it hooks). Its spells have
    `ThreatMultiplier` 0; the DK's threat is his own damage's.
