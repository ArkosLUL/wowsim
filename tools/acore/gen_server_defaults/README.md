# gen_server_defaults

Generates the live server's value for every `proto.ServerSettings` field:
`sim/core/server_defaults_auto_gen.go` (`LiveServerDefaults()` and the dungeon scale floors) and
`ui/core/constants/server_defaults_auto_gen.ts` (`LIVE_SERVER_DEFAULTS`).

```bash
tools/acore/dock.sh run ./tools/acore/gen_server_defaults
```

Rerun it whenever the live config changes. It imports only `sim/core/proto`, so after changing
`ServerSettings` run `dock.sh proto` first; `sim/core` needn't compile.

Inputs, under `-ac` (default `/ac`, dock.sh's mount of the AzerothCore checkout):
- `worldserver.conf.dist` and the conf.dist files of mod-spell-tweaks, mod-dungeon-scale and
  mod-reforging;
- `configurationOverrides/{ServerPerformance,SpellTweaks,DungeonScale}.env`. The other override files
  hold credentials and unrelated settings and are never opened, so a key it needs set in one of them is
  missed.

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
  creature beyond the module's stock list and the mod-sim-validation dummies (999000-999003).

`TestCodeDefaultsMatchServerSource` checks its keys and code defaults against the server source, and
`TestGeneratedFilesMatchLiveConfig` the committed output against the live config.
