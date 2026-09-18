# Item availability comes from a server-derived catalog

Which items and gems exist in each content phase comes from a catalog generated from the live server: loot,
vendor, quest and crafting tables, mapped to mod-individual-progression's progression tiers. The catalog also
carries equip limits and faction. The BiS optimizer and the gear picker both use it. This amends
[ADR 0002](0002-item-data-from-live-db.md): Wowhead and AtlasLoot phases and sources stay for display only.

Classic phases with a list of fixes was rejected. Classic tags Ulduar-10 loot as Titan Rune drops, which the
server doesn't have. It puts epic gems in phase 3, but the server drops them from Titanium Ore prospecting
at any tier. And its enchant phases are unusable.
