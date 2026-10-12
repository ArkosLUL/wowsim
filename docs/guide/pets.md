# Pets and guardians

How a player's summons follow the server: stats, timers, the buffs and casts that reach them, and the
guardians' scripted AIs. Declaring their spells: [spell data and timing](spell-data-and-timing.md). The files:
[sim architecture](sim-architecture.md#classes-items-encounters).

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
- Real pets rescale every 2 s, Spirit Wolves every 1 s, other guardians once at the summon.
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
- Wrath of Air is a raid area aura (effect 65), so it reaches a guardian too. The sim gives it only to a pet
  with `Pet.TakesWrathOfAir`, the Greater Fire Elemental; the Imp, Water Elemental and Mirror Images go
  without.
- Haste carriers (the hunter pet, the DK's ghouls, gargoyle and army, Spirit Wolves; `Unit.HasteCarrier`)
  ignore direct haste and slows, Bloodlust included, and take the owner's attack speed as whole percent
  (`Pet.OwnerHasteSource`): at the summon, then every 2 s for a pet and for Spirit Wolves
  (`Pet.OwnerHasteTicksOnGuardian`), never again for the DK's guardians.

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
  - Summoned before the pull, they open on either spell, evenly (`CasterAI`).
  - Summoned into a fight, they're engaged at the summon with no victim to open on: a Frostbolt an update
    after the wait, and 3 Fire Blasts a life.
