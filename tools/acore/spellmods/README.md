# spellmods

Writes `sim/core/testdata/simval/spellmod_spells.json`: for every implemented talent whose rank spells carry
spell mods, the spells the sim names that each mod changes on the server. The e2e's spell-mod sweep probes them
before and after the talent. Offline: reads the committed spelldump and `implemented_talents.json`, and scans
`sim/` for spell ids as [spellids](../spellids/README.md) does.

```bash
tools/acore/dock.sh run ./tools/acore/spellmods
```

| Flag | Default |
|---|---|
| `-root` | `.`, the checkout to scan |
| `-dump` | `assets/db_inputs/acore/spelldump.jsonl` |
| `-talents` | `sim/core/testdata/simval/implemented_talents.json` |
| `-out` | `sim/core/testdata/simval/spellmod_spells.json` |

Rerun after changing either input or adding a spell id to `sim/`; a rerun rewrites the file byte for byte.

## Rule

- Mods: each effect with aura 107 (flat) or 108 (pct), op from `miscValue`, mask from `classMask`, family from
  the rank spell (`AuraEffect::CalculateSpellMod`).
- A spell matches by `SpellInfo::IsAffected`: family 0 matches all; else same family, and an empty mask or one
  sharing a bit with `familyFlags`.
- Excluded, listed under `excluded`: duration mods on infinite-duration spells (`Player::IsAffectedBySpellmod`),
  any other op on a spell with `SPELL_ATTR3_IGNORE_CASTER_MODIFIERS` (`SpellInfo::IsAffectedBySpellMods`).
- Kept only when the sim names the spell in `sim/<class>/` or outside every class package: the Felguard's
  Cleave (warrior family) never takes warrior mods.
- Script exceptions, in `spellModScripts`: `DoEffectCalcSpellMod` scripts that build or rewrite a mod. Improved
  Demonic Tactics builds flat mods on its dummy effects (pet passives); any other such script on a rank spell
  fails the generator. No server code hooks `OnIsAffectedBySpellModCheck`. `TestSpellModScriptsMatchServer`
  checks both against the server source at `/ac` (skipped without it).

Runtime conditions the list can't decide: a cast talent (`passive: false`) applies its mods only while its aura
is up; `equipped` names the item the aura needs; forms and stances come from the talent path's auras; pet spells
(Claw, Firebolt) take the owner's mods but need a pet probe.

## Output

```
{"talents": [{"class", "tree", "talent", "passive", "equipped"?, "groups": [
  {"ranks": [rank spell ids], "mods": [{"effect", "type", "op", "classMask", "script"?}],
   "spells": [{"id", "name", "ops"}], "excluded"?: [{"id", "name", "op", "reason"}]}]}]}
```

`talent` is the tree file's `fieldName`, as in `talent_paths.json`. A group holds the ranks with the same mods,
values aside; a few talents' ranks differ in mask or op in the DBC, so look a path step up by its rank spell.
`op` is `SpellModOp` without `SPELLMOD_`.
