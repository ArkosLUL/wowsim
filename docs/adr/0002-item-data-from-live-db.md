# Item stats come from the live server DB

Item stats are generated from the live `acore_world` DB, module SQL and manual tweaks included, by a
`gen_db` AzerothCore mode built on `azerothcore.ConvertItem`. That covers item level, quality, stats,
sockets, socket bonus, weapon damage and speed, heroic flag, class allowlist and set name. An override
list was rejected: about 880 items differ in item level alone, so it would be a second DB to maintain. Wowhead and AtlasLoot
data stay for icons, phases, types and sources. Item effects and set bonuses stay hand-written in Go and
are corrected from the item-diff report.
