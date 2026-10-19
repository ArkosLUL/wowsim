# gen_server_defaults

Generates the live server's value for every `proto.ServerSettings` field, plus the limits the server
enforces on them: `sim/core/server_defaults_auto_gen.go` (`LiveServerDefaults()`, the dungeon scale
floors and the mod-reforging limits) and `ui/core/constants/server_defaults_auto_gen.ts`
(`LIVE_SERVER_DEFAULTS` and the same limits). Also the playerbots' level 80 flask, elixirs and food
per class and spec, `sim/core/playerbot_world_buffs_auto_gen.go`
([playerbot rules](../../../docs/guide/playerbot-rules.md)).

```bash
tools/acore/dock.sh run ./tools/acore/gen_server_defaults
```

Rerun it whenever the live config or the bots' world-buff matrix changes. It imports only
`sim/core/proto`, so after changing `ServerSettings` run `dock.sh proto` first; `sim/core` needn't
compile.

Inputs, under `-ac` (default `/ac`, dock.sh's mount of the AzerothCore checkout):
- `worldserver.conf.dist` and the conf.dist files of mod-spell-tweaks, mod-dungeon-scale and
  mod-reforging;
- `configurationOverrides/{ServerPerformance,SpellTweaks,DungeonScale}.env`, and `Playerbot.env` for
  `AC_PLAYERBOTS_WORLD_BUFF_MATRIX` alone (else `Playerbots.WorldBuffMatrix` in mod-playerbots'
  conf.dist). The other override files hold credentials and unrelated settings and are never opened, so
  a key it needs set in one of them is missed;
- `modules/mod-reforging/src/item_reforge.h`, for the percentage range, the default percentage and stat
  list, and the stat-list length cap. A renamed constant there fails the run rather than falling back.

Each key resolves as worldserver's `GetOption` does: the `AC_` variable, then the conf file, then the
code default; a value that doesn't parse takes the default. Dungeon scale 10M/25M keys left blank stay
unset, and the sim falls back to the generic raid set.

worldserver really loads `env/dist/etc/modules/*.conf`, copied once from conf.dist and never updated.
So the generator rebuilds from those (a missing one means code defaults) and writes nothing if the
result differs; without that directory it skips the check. The installed `worldserver.conf` holds
credentials and is never opened; its conf.dist stands in.

It writes nothing when the dungeon scale config does something the sim doesn't model for a full WotLK
raid:
- scaling off for a raid size, globally, or autoscale off;
- a raid player-count curve other than floor 0, ceiling 1, or a player-count offset;
- a per-instance override or disable naming a raid map;
- a per-creature override: any `StatModifier.PerCreature` or `ForcedID` entry, or a `DisabledID`
  creature beyond the module's stock list and the mod-sim-validation dummies (999000-999008).

It also writes nothing when a level 80 matrix row holds a spell the sim has no flask, elixir or food
for, two buffs for one consumable slot, or differs between the factions.

`TestCodeDefaultsMatchServerSource` checks its keys and code defaults against the server source, and
`TestGeneratedFilesMatchLiveConfig` the committed output against the live config.
