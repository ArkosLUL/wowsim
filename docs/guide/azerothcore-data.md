# AzerothCore data

Table and DBC facts for reading server data. The readers are in `tools/database/azerothcore/`. DBC
layouts come from [ac] `src/server/shared/DataStores/DBCStructure.h` and `DBCfmt.h`.

## Overrides

- `acore_world.*_dbc` tables override same-id DBC rows on the server, so read both. For example,
  `spell_dbc` overrides Spell.dbc.
- Don't use the `SpellDB`/`SpellDBServer` databases: they're a spell-editor import.
- Missing module tables raise MySQL error 1146. Warn, don't fail.

## Base stats

- `acore_world.player_class_stats` (per class and level: BaseHP, BaseMana, the five stats) plus
  `player_race_stats` (offsets).
- gt*.dbc: the Clean, Changed and live copies are byte-identical.
  - Most hold one float per row, indexed by row: `cr·100 + level − 1` in gtCombatRatings,
    `(class − 1)·100 + level − 1` in gtChanceTo{Melee,Spell}Crit and gtRegenMPPerSpt, `class − 1` in the
    `*Base` tables.
  - gtOCTClassCombatRatingScalar is keyed by its id column: `(class − 1)·32 + cr + 1`.

## Items (`acore_world`)

- `item_template`:
  - Some numeric columns are NULL, so COALESCE them.
  - `armor` is the total, and `ArmorDamageModifier` is its bonus part.
  - `block` only applies to shields.
  - `VerifiedBuild = 15595` marks placeholder rows.
  - Per-item procs are in `spellppmRate_N` and `spellcooldown_N`.
- An item is obtainable when any of these reference it:
  - `*_loot_template` with `Reference = 0`
  - `npc_vendor` or `game_event_npc_vendor`
  - `achievement_reward`
  - `quest_template` `RewardItem*` or `RewardChoiceItemID*`
- 10- and 25-man tier pieces share one ItemSet.

## Spells

- `spell_cooldown_overrides`.
- `spell_proc`: a negative `SpellId` applies to the whole rank chain.
- `spell_enchant_proc_data`.
- Spell.dbc field indices: ProcChance 35, DurationIndex 40, Effect 71, DieSides 74, BasePoints 80,
  Aura 95, ItemType 107, MiscValue 110, TriggerSpell 116, Name 136.
- Cooldown and PPM precedence follow `Player::CastItemCombatSpell` and
  `Player::AddSpellAndCategoryCooldowns`.
- Proc-triggered spells and passive auras never start cooldowns (`Spell::SendSpellCooldown`).

## Characters (`acore_characters`)

- **Gear:** `character_inventory` with `bag = 0 AND slot < 19`, minus 3 (shirt) and 18 (tabard), joined
  to `item_instance`.
- **`item_instance.enchantments`** is 36 space-separated tokens:
  - 0 is the permanent enchant.
  - 6, 9 and 12 are the gem enchants. Map each to its gem item through SpellItemEnchantment.dbc field 33.
  - 18 is the socket-adding enchant (belt buckle 3729, Blacksmithing 3717/3723). Never emit it.
- **`randomPropertyId`:** above 0 it's a random property; below 0, a random suffix.
- **Talents:** `character_talent(spell, specMask)`. Keep rows where `specMask & (1 << activeTalentGroup)`
  is set; only the active spec is exported.
  - Positions come from Talent.dbc (0 id, 1 tab, 2 row, 3 col, 4–8 rank spells) and TalentTab.dbc
    (20 ClassMask, 22 tab page).
  - Use the DBC files, not `acore_world.talent_dbc`, which carries mod-spell-tweaks' hunter tier swap.
- **Glyphs:** `character_glyphs(talentGroup, glyph1..6)`, mapped through GlyphProperties.dbc
  (1 SpellID; 2 flags, 0 major and 1 minor).
- **Group:** `group_member(guid, memberGuid, memberFlags, subgroup)` joined to `` `groups`.leaderGuid ``.
  - Backtick `groups`: it's a reserved word.
  - memberFlags: 1 assistant, 2 main tank, 4 main assist.
  - `groupType & 2` marks a raid.
  - The table is empty while no group exists.

## Professions

- Skills: `character_skills(skill, value)`. Primary skill ids: 164, 165, 171, 182, 186, 197, 202, 333,
  393, 755, 773.
- Profession spells live in two tables, so read both:
  - `character_spell(guid, spell)`
  - `shared_professions_account_spells(account_id, spell_id)`, joined through `characters.account`.
    mod-shared-professions applies these at login and never copies them to `character_spell`.
- Skill value doesn't tell you who has a profession bonus. Playerbots got theirs from the user's GM
  `.learn` (in `character_spell`); human-played characters get theirs through the account table. The
  bonus spells:

  | Spell | Id |
  |---|---|
  | Toughness r6 (+60 Sta) | 53040 (r1–r5: 53120–53124) |
  | Master of Anatomy r6 (+40 crit) | 53666 (r5: 53665) |
  | Mixology | 53042 |
  | Lifeblood | 55503 |

## Module tables

- `character_reforging(guid, item_guid, stat_decrease, stat_increase, stat_value)`. `stat_value` is
  floor(40% of the server's stat), so it can be 1–2 off the sim's. The sim recomputes from its own item.
- `character_racial_swap(guid, selected_race)`.
