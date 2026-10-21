# Items, gems and enchants

Where the sim's items, gems and enchants come from, and how their effects and item swaps follow the server.
The core rules they run on (procs, costs, hit rules): [server parity](server-parity.md).

## Item data

`db.json` takes item stats, ilvls, sockets and set names from the server (`make items`,
[ADR 0002](../adr/0002-item-data-from-live-db.md)). acdiff ([README](../../tools/database/acdiff/README.md))
checks it and the Go effects.

- **Kept from Wowhead:** items nothing on the server awards (the only rows left in `items_diff.csv`),
  heirlooms and random-enchant items.
- Verdicts per row are in `docs/azerothcore-item-diff/data/{effects,sets}_review.csv`.

## Item effects

- Shared ones (`sim/common`) match the server. Thunderfury's and Rod of the Sun King's PPMs are literals:
  `serverdata` doesn't carry `item_template`'s `spellppmRate`.
- Item damage is dealt as the item's server spell, with its range, whole rolls and missile travel, so class
  auras read its damage class and cast time: the proc trinkets, capacitors and Gnomish Lightning Generator
  (`itemDamageSpell` in `sim/common/wotlk`), and the explosives by their use spells (56350, 67769, 56488;
  `sim/core/consumes.go`). `legacyAPLItemIDs` keeps a saved rotation that names the item working.
- Item heals go the same way (`itemHealSpell`): Bauble of True Blood's use casts 71607 for both items,
  Althor's Abacus heals as 71610 / 71641, Glowing Twilight Scale ticks as 75493 / 75494. Still under the
  item: Forethought Talisman's heal over time.
- Althor's Abacus heals the enabled, [living](damage-and-healing.md#healing) raid unit with the lowest health
  percent (`spell_item_echoes_of_light`), a tie at random, no one when none is left; a unit without a health bar
  (a target dummy) counts as full.
- Bryntroll's proc is Drain Life 71838 (heroic 71839), magic class. It never crits (`SPELL_ATTR2_CANT_CRIT`)
  and heals the wielder for the damage dealt, a bare `HealBySpell` with no crit, threat, proc or spell power
  (`GainHealing`). The item gives the spell a 500 ms cooldown, so a roll won inside it is lost
  (`Player::CastItemCombatSpell`).
- Shard of the Gods: the Purified set (bonus 69755) casts only Cauterizing Heal 69733, the Shiny set (69739)
  only Searing Flames 69729, with no heroic ids: 25%, 50 s cooldown.
  - It rolls once for every target a magic-class cast lands on, whatever the cast did there, triggered casts
    too (`SPELL_ATTR3_CAN_PROC_FROM_PROCS`), never a tick or a melee or ranged spell
    ([landed casts](server-parity.md#other-measured-server-behaviour); `rollsShardOfTheGods`).
  - The class is the landing spell's: one with no damage class never rolls (Ardent Defender 66233, Power
    Infusion, Invigorating Earthsiege Health Regen 55341, the item hots, Persistent Shield). A sim spell
    landing as another id maps it in `shardLandingSpellIDs` (keep the name: `spellids` reads it for
    `gen_serverdata`); with no server data under its action id (a glyph's heal under its item), the proc mask
    decides.
  - A landing of the wrong kind uses the proc up and casts nothing (`SPELL_FAILED_BAD_TARGETS`).
- Class relics, sigils, totems, idols and a few class set bonuses still carry Classic values until their
  class's P7 item, except Idol of the Ravenous Beast ([caster mechanics](caster-mechanics.md#druid)). Totem of
  the Third Wind also buffs Healing Wave.

## Gems and enchants

- **Meta gems** work only with their `SpellItemEnchantmentCondition` met (`sim/core/meta_gems.go`); an inactive
  one gives nothing. An item swap that meets or breaks the condition moves the whole enchant
  (`Player::CorrectMetaGemEnchants`; `metaGemSwap`): the stats, the effect (its `OnMetaGemSwap` toggles, for a
  gem registered with `NewMetaGemEffect`) and every crit multiplier, heals' or damage's by the gem. Class code
  that touches a crit multiplier mid-fight has to follow the swap: a factor held on it goes through
  `Character.ScaleCrit` (Combustion, Hodir's Storm Power), a value worked out ahead of time through
  `MetaCritFollow` and `MetaCritBuilt` (Prey on the Weak, Predatory Instincts).
- **Enchant PPMs:** Mongoose 1, Icebreaker 3, Deathfrost 3. An enchant procs only from the weapon that hit.

## Item swaps

The server's swap takes the leaving weapon's enchants off and puts the new one's on (`Player::SwapItem`,
`_ApplyItemMods`). In the sim (`sim/core/item_swaps.go`):
- An enchant's item stats move with the item. A weapon enchant's static effect moves through
  `NewEnchantSwapEffect`, counted per worn weapon (the Death Knight runes, Scourgebane, the scopes, Sunfire,
  Soulfrost; `NewEnchantSwapMultiplyStat` for a multiplied stat), a weapon damage enchant through
  `swapWeapon`.
- An enchant whose effect a swap callback turns on and off goes in `swapFollowingEnchants`, or the weapon in
  the bag would apply it beside the worn one's: the two `RegisterOnSwapItemFor…` helpers panic on a missing
  id, a direct `RegisterOnItemSwap` doesn't (Deathfrost, Razorice).
- `ItemSwap.reset` right after `Unit.reset` puts the main set's gear back and runs the swap callbacks, without
  `SwapItems`' stat change and GCD; the APL's mid-fight swap to the main set is a full `SwapItems`.
- A racial weapon expertise (a human's sword or mace) follows the swap per hand (`followWeaponSpecialization`;
  `Player::UpdateExpertise`).
